/**
 * token_frames.js -- the thirty frames of the token maker, drawn in code on
 * canvas (window.ChronicleTokenFrames).
 *
 * The token maker (token_maker.js) loads this file the first time it opens,
 * so a page pays for it only when someone makes a token. Nothing here touches
 * the page: it draws one token into a canvas it is handed. The materials are
 * worked out once at a master size and cached; the moving frames compute one
 * small texture per frame and draw it scaled. Each design names its own main
 * colour, so the colour control can turn any of them to the creature's colour.
 *
 * After adding, removing or reordering a design, run
 * `node tools/token_frames_previews.mjs` to redraw the picker's previews.
 */
(function () {
  'use strict';

  // Placement is in work units: a square work surface W across, the frame
  // {x, y, w, h} and the picture {x, y, s} (its centre and scale) on it.
  var W = 1100;

  // The state the drawing reads. render() below fills it from what it is
  // handed, so one engine draws any token.
  var S = { mode: 'token', img: null, flip: false, shape: 'round', token: null, name: '' };
  var EXTRA = []; // designs that live in their own sections register here (see The set)
  function P() { return S[S.mode]; }

  /* ---------- Sample art: a backlit, painterly figure, drawn in code so the previews need no picture file.
     It is drawn as two layers, background and subject, the way token art is cut for pop-outs. ---------- */
  var ART_HEAD = [600, 470]; // where the sample's head is, in its own pixels
  function sampleArt() {
    var Wd = 1200, Hd = 1600;
    function layer() { var c = document.createElement('canvas'); c.width = Wd; c.height = Hd; return c; }
    var bgc = layer(), subc = layer(), g = bgc.getContext('2d');
    function blob(x, y, r, col, blur) { g.save(); g.filter = 'blur(' + blur + 'px)'; g.fillStyle = col; g.beginPath(); g.arc(x, y, r, 0, 7); g.fill(); g.restore(); }
    var bg = g.createLinearGradient(0, 0, 0, Hd); bg.addColorStop(0, '#1d2326'); bg.addColorStop(0.5, '#141819'); bg.addColorStop(1, '#0b0c0d');
    g.fillStyle = bg; g.fillRect(0, 0, Wd, Hd);
    blob(640, 470, 330, 'rgba(176,150,112,.55)', 90);
    blob(600, 430, 140, 'rgba(232,214,180,.55)', 50);
    blob(250, 1200, 380, 'rgba(48,66,64,.6)', 120);
    blob(1000, 1050, 320, 'rgba(60,58,52,.5)', 120);
    g = subc.getContext('2d');
    g.save(); g.filter = 'blur(2px)';
    var fig = g.createLinearGradient(0, 380, 0, Hd); fig.addColorStop(0, '#2a2c31'); fig.addColorStop(0.5, '#16171a'); fig.addColorStop(1, '#0a0a0b');
    g.fillStyle = fig; g.beginPath(); g.moveTo(600, 360); g.bezierCurveTo(430, 365, 400, 600, 430, 760);
    g.bezierCurveTo(260, 880, 160, 1250, 120, 1600); g.lineTo(1100, 1600); g.bezierCurveTo(1050, 1250, 950, 880, 780, 760);
    g.bezierCurveTo(805, 600, 770, 365, 600, 360); g.fill(); g.restore();
    // A spiked iron crown on the hood, dark against the light with a warm edge so it reads at token size.
    g.save(); g.filter = 'blur(1px)';
    g.beginPath(); g.moveTo(488, 410);
    [[504, 290], [532, 372], [558, 250], [586, 364], [600, 222], [614, 364], [642, 250], [668, 372], [696, 290], [712, 410]].forEach(function (q) { g.lineTo(q[0], q[1]); });
    g.quadraticCurveTo(600, 436, 488, 410); g.closePath();
    var cg = g.createLinearGradient(0, 222, 0, 436); cg.addColorStop(0, '#3a3530'); cg.addColorStop(1, '#141416');
    g.fillStyle = cg; g.fill(); g.lineJoin = 'round'; g.strokeStyle = 'rgba(214,184,132,.6)'; g.lineWidth = 5; g.stroke();
    g.restore();
    g.save(); g.globalCompositeOperation = 'source-atop'; g.filter = 'blur(6px)'; g.strokeStyle = 'rgba(214,190,150,.55)'; g.lineWidth = 14;
    g.beginPath(); g.moveTo(598, 366); g.bezierCurveTo(440, 372, 410, 600, 438, 760); g.stroke(); g.restore();
    [[562, 560], [640, 560]].forEach(function (e) {
      g.save(); g.globalCompositeOperation = 'lighter'; g.filter = 'blur(5px)';
      g.fillStyle = 'rgba(170,205,220,.55)'; g.beginPath(); g.ellipse(e[0], e[1], 14, 6, 0, 0, 7); g.fill();
      g.filter = 'blur(1px)'; g.fillStyle = 'rgba(235,245,250,.9)'; g.beginPath(); g.ellipse(e[0], e[1], 5, 2.4, 0, 0, 7); g.fill(); g.restore();
    });
    [bgc, subc].forEach(function (c, li) {
      var x = c.getContext('2d'), img = x.getImageData(0, 0, Wd, Hd), d = img.data, seed = 7 + li;
      for (var i = 0; i < d.length; i += 4) { seed = (seed * 16807) % 2147483647; if (!d[i + 3]) continue; var n = ((seed / 2147483647) - 0.5) * 18; d[i] += n; d[i + 1] += n; d[i + 2] += n; }
      x.putImageData(img, 0, 0);
    });
    var full = layer(), f = full.getContext('2d');
    f.drawImage(bgc, 0, 0); f.drawImage(subc, 0, 0);
    var v = f.createRadialGradient(600, 650, 300, 600, 800, 1000); v.addColorStop(0, 'rgba(0,0,0,0)'); v.addColorStop(1, 'rgba(0,0,0,.65)');
    f.fillStyle = v; f.fillRect(0, 0, Wd, Hd);
    return { full: full, sub: subc };
  }
  function iw() { return S.img.naturalWidth || S.img.width; }
  function ih() { return S.img.naturalHeight || S.img.height; }

  /* ---------- Placement: frame {x,y,w,h} and picture {x,y,s}, in work units ---------- */
  function aspectOf(mode) { return mode === 'token' ? 1 : S.aspect; }
  function startPlacement(mode) {
    var a = aspectOf(mode), fw = a >= 1 ? W * 0.6 : W * 0.6 * a, fh = fw / a;
    var s = Math.min(W * 0.9 / iw(), W * 0.9 / ih());
    S[mode] = { img: { x: W / 2, y: W / 2, s: s }, fr: { x: W / 2, y: mode === 'token' ? W * 0.43 : W / 2, w: fw, h: fh } };
  }
  function fillFrame() {
    var p = P(), s = Math.max(p.fr.w / iw(), p.fr.h / ih());
    p.img = { x: p.fr.x, y: p.fr.y, s: s };
  }
  function keepFrameIn() {
    var fr = P().fr, max = W * 0.98;
    var k = Math.min(1, max / fr.w, max / fr.h); fr.w *= k; fr.h *= k;
    fr.w = Math.max(fr.w, 60); fr.h = fr.w / aspectOf(S.mode);
    fr.x = Math.min(W - fr.w / 2, Math.max(fr.w / 2, fr.x));
    fr.y = Math.min(W - fr.h / 2, Math.max(fr.h / 2, fr.y));
  }
  // The head marker moves with the picture, so it is stored relative to the picture's centre and scale.

  function rgb(hex) { var n = parseInt(hex.slice(1), 16); return [n >> 16 & 255, n >> 8 & 255, n & 255]; }
  function rgba(c, a) { return 'rgba(' + Math.round(c[0]) + ',' + Math.round(c[1]) + ',' + Math.round(c[2]) + ',' + a + ')'; }
  function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
  function smooth(a, b, x) { var t = clamp((x - a) / (b - a), 0, 1); return t * t * (3 - 2 * t); }
  function bead(a, b, x) { if (x < a || x > b) return 0; var t = (x - a) / (b - a) * 2 - 1; return Math.sqrt(1 - t * t); }
  function lerp(a, b, t) { return a + (b - a) * t; }
  function mix3(a, b, t) { return [lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)]; }

  /* ---------- Recolouring ----------
     Any design can take a new colour. Each names its own main colour (key) and how far its lightness follows the
     new one (luma: 0 keeps fire bright whatever colour it turns, 1 lets black ink turn white). Colour is turned
     and scaled in the YIQ chroma plane so the key lands on the new colour, and lightness is bent by a curve, so
     the shading, grain and variation inside a design survive. A grey key has no hue to turn, so it is tinted. */
  var GRADE = null; // the grade in force while a design is drawn; null draws it in its own colours
  function yiq(c) { var r = c[0] / 255, g = c[1] / 255, b = c[2] / 255; return [0.299 * r + 0.587 * g + 0.114 * b, 0.596 * r - 0.274 * g - 0.322 * b, 0.211 * r - 0.523 * g + 0.312 * b]; }
  // spread: how much of the hue variation inside a design survives (fire keeps little of its red-to-yellow, or blue
  // fire would run to violet in its hot cores).
  function makeGrade(key, target, luma, spread) {
    var k = yiq(key), t = yiq(target), ck = Math.hypot(k[1], k[2]), ct = Math.hypot(t[1], t[2]);
    var zr = 0, zi = 0, tint = 1 - smooth(0.02, 0.06, ck);          // how much of the change is a tint rather than a turn
    if (tint < 1) { zr = (t[1] * k[1] + t[2] * k[2]) / (ck * ck); zi = (t[2] * k[1] - t[1] * k[2]) / (ck * ck); var zl = Math.hypot(zr, zi); if (zl > 3) { zr *= 3 / zl; zi *= 3 / zl; } }
    var yk = clamp(k[0], 0.02, 0.98), yt = clamp(lerp(k[0], t[0], luma || 0), 0.02, 0.98), gam = Math.log(yt) / Math.log(yk);
    spread = spread == null ? 1 : spread;
    return { id: key.join(',') + '>' + target.join(',') + '@' + (luma || 0) + '~' + spread, target: target, zr: zr, zi: zi, tint: tint, ti: t[1], tq: t[2], gam: gam,
      ux: ck ? k[1] / ck : 1, uy: ck ? k[2] / ck : 0, spread: spread };
  }
  function gradeYIQ(g, Y, I, Q, out, o) {
    var y = Math.pow(Math.max(0, Y), g.gam), par = I * g.ux + Q * g.uy, perp = (Q * g.ux - I * g.uy) * g.spread;
    var Is = par * g.ux - perp * g.uy, Qs = par * g.uy + perp * g.ux;
    var turnI = g.zr * Is - g.zi * Qs, turnQ = g.zi * Is + g.zr * Qs, w = Math.min(1, y * 2.2);
    var i2 = turnI + (I + g.ti * w - turnI) * g.tint, q2 = turnQ + (Q + g.tq * w - turnQ) * g.tint;
    out[o] = 255 * (y + 0.956 * i2 + 0.621 * q2); out[o + 1] = 255 * (y - 0.272 * i2 - 0.647 * q2); out[o + 2] = 255 * (y - 1.106 * i2 + 1.703 * q2);
  }
  var TC = [0, 0, 0];
  // One colour under the grade in force: every colour an effect draws with goes through this.
  function tc(c) {
    if (!GRADE) return c;
    var r = c[0] / 255, g = c[1] / 255, b = c[2] / 255;
    gradeYIQ(GRADE, 0.299 * r + 0.587 * g + 0.114 * b, 0.596 * r - 0.274 * g - 0.322 * b, 0.211 * r - 0.523 * g + 0.312 * b, TC, 0);
    return [clamp(TC[0], 0, 255), clamp(TC[1], 0, 255), clamp(TC[2], 0, 255)];
  }
  function gradeId() { return GRADE ? GRADE.id : ''; }
  // A whole layer under a grade, as a new canvas.
  function gradeCanvas(src, g) {
    var c = document.createElement('canvas'); c.width = src.width; c.height = src.height;
    var x = c.getContext('2d'); x.drawImage(src, 0, 0);
    var img = x.getImageData(0, 0, c.width, c.height), d = img.data;
    for (var o = 0; o < d.length; o += 4) {
      if (!d[o + 3]) continue;
      var r = d[o] / 255, gg = d[o + 1] / 255, b = d[o + 2] / 255;
      gradeYIQ(g, 0.299 * r + 0.587 * gg + 0.114 * b, 0.596 * r - 0.274 * gg - 0.322 * b, 0.211 * r - 0.523 * gg + 0.312 * b, d, o);
    }
    x.putImageData(img, 0, 0);
    return c;
  }
  // A layer under the grade in force, kept until the layer itself is redrawn.
  function graded(key, src) {
    if (!GRADE) return src;
    var k = key + '|' + src.width + 'x' + src.height + '#' + GRADE.id, c = gradedCache.get(k);
    if (c && c.src === src) return c.cv;
    if (gradedCache.size > 120) gradedCache.clear();
    c = { src: src, cv: gradeCanvas(src, GRADE) }; gradedCache.set(k, c); return c.cv;
  }
  var gradedCache = new Map();

  /* ---------- Shapes ---------- */
  function round() { return S.mode === 'token' ? S.shape === 'round' : false; }
  function shapePath(ctx, w, h, inset, isRound) {
    ctx.beginPath();
    if (isRound) { ctx.arc(w / 2, h / 2, Math.max(0.5, Math.min(w, h) / 2 - inset), 0, Math.PI * 2); return; }
    ctx.rect(inset, inset, w - inset * 2, h - inset * 2);
  }

  /* ---------- Noise ---------- */
  function hash(x, y) { var n = Math.sin(x * 127.1 + y * 311.7) * 43758.5453; return n - Math.floor(n); }
  // Lattice values come from a table so the per-pixel materials stay fast enough to draw at page load.
  var HT = new Float32Array(65536);
  (function () { for (var i = 0; i < 65536; i++) HT[i] = hash(i % 256 + 0.37, Math.floor(i / 256) + 0.71); })();
  function lat(x, y) { return HT[((y & 255) << 8) | (x & 255)]; }
  function vnoise(x, y) {
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi;
    var a = lat(xi, yi), b = lat(xi + 1, yi), c = lat(xi, yi + 1), d = lat(xi + 1, yi + 1);
    var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
    return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v;
  }
  function fbm(x, y, oct) {
    var s = 0, a = 0.5, n = oct || 4;
    for (var i = 0; i < n; i++) { s += a * vnoise(x, y); x = x * 2.03 + 17.1; y = y * 2.01 + 9.7; a *= 0.5; }
    return s / (1 - Math.pow(0.5, n));
  }
  // Value noise that repeats every per cells along x (a power of two), so noise wrapped round a ring has no seam.
  function pvn(x, y, per) {
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi, m = per - 1, x0 = xi & m, x1 = (x0 + 1) & m;
    var a = lat(x0, yi), b = lat(x1, yi), c = lat(x0, yi + 1), d = lat(x1, yi + 1);
    var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
    return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v;
  }
  function pfbm(x, y, per, oct) {
    var s = 0, a = 0.5, n = 0;
    for (var i = 0; i < oct; i++) { s += a * pvn(x, y, per); n += a; x *= 2; y = y * 2.01 + 9.7; per *= 2; a *= 0.5; }
    return s / n;
  }
  // Cellular noise: distance to the nearest and second-nearest cell points, and the nearest cell's id and centre.
  var VOR = { f1: 0, f2: 0, id: 0, cx: 0, cy: 0 };
  function voronoi(x, y) {
    var xi = Math.floor(x), yi = Math.floor(y), f1 = 9, f2 = 9, id = 0, cx = 0, cy = 0;
    for (var j = -1; j <= 1; j++) for (var i = -1; i <= 1; i++) {
      var gx = xi + i, gy = yi + j, px = gx + lat(gx, gy), py = gy + lat(gy + 97, gx + 41);
      var d = Math.hypot(px - x, py - y);
      if (d < f1) { f2 = f1; f1 = d; id = lat(gx + 13, gy + 177); cx = px; cy = py; } else if (d < f2) f2 = d;
    }
    VOR.f1 = f1; VOR.f2 = f2; VOR.id = id; VOR.cx = cx; VOR.cy = cy; return VOR;
  }

  /* ---------- Rim shader ----------
     A material is a height field plus a colour rule. Pass one samples the height over the rim (NaN where
     the material is missing, for chips and torn edges); pass two takes real 2D normals from that height, so
     cracks, facets and carving catch the light, then lights it from the top left. Nothing is outlined.
     Coordinates handed to the material are size-independent: X, Y in -0.5..0.5 across the token, A along
     the rim and r in from the outer edge, both as a fraction of the token size, and u across the rim 0..1. */
  var LIGHT = (function () { var l = [-0.55, -0.68, 0.46], n = Math.hypot(l[0], l[1], l[2]); return l.map(function (v) { return v / n; }); })();
  var HALF = (function () { var h = [LIGHT[0], LIGHT[1], LIGHT[2] + 1], n = Math.hypot(h[0], h[1], h[2]); return h.map(function (v) { return v / n; }); })();
  function diffuse(N) { return Math.max(0, N[0] * LIGHT[0] + N[1] * LIGHT[1] + N[2] * LIGHT[2]); }
  function specular(N, shine) { return Math.pow(Math.max(0, N[0] * HALF[0] + N[1] * HALF[1] + N[2] * HALF[2]), shine); }

  var layerCache = new Map(), masterCache = new Map();
  // Detail finer than about three pixels can't be drawn without shimmer, so materials fade it out with bl().
  var PXF = 1 / 400;
  function bl(freq) { return 1 - smooth(0.12, 0.33, freq * PXF); }
  // Every layer is shaded once, at a master size, and smaller copies are scaled down from it in halving steps,
  // so the map, the hover card and the frame list cost almost nothing and fine detail averages instead of shimmering.
  var MASTER = 720;
  function scaleDown(src, w, h) {
    var cur = src;
    while (cur.width >= w * 2 && cur.height >= h * 2) {
      var c = document.createElement('canvas'); c.width = Math.round(cur.width / 2); c.height = Math.round(cur.height / 2);
      var g = c.getContext('2d'); g.imageSmoothingQuality = 'high'; g.drawImage(cur, 0, 0, c.width, c.height); cur = c;
    }
    var out = document.createElement('canvas'); out.width = w; out.height = h;
    var o = out.getContext('2d'); o.imageSmoothingQuality = 'high'; o.drawImage(cur, 0, 0, w, h);
    return out;
  }
  function masterDims(w, h) {
    var r = Math.round(w / h * 100) / 100;
    return r >= 1 ? [MASTER, Math.round(MASTER / r)] : [Math.round(MASTER * r), MASTER];
  }
  // A cached layer of any kind: draw(w, h) makes it at the master size, smaller sizes are scaled from that.
  function masterLayer(key, w, h, draw) {
    var k = key + '|' + w + '|' + h, m = masterDims(w, h), isMaster = w === m[0] && h === m[1];
    var cache = isMaster ? masterCache : layerCache;
    if (cache.has(k)) return cache.get(k);
    if (layerCache.size > 160) layerCache.clear();
    var cv = isMaster || w > m[0] ? draw(w, h) : scaleDown(masterLayer(key, m[0], m[1], draw), w, h);
    cache.set(k, cv);
    return cv;
  }
  function rimLayer(id, w, h, isRound, mat) {
    return masterLayer(id + '|' + isRound, w, h, function (W2, H2) { return shadeRim(W2, H2, isRound, mat); });
  }
  function boxBlur(src, w, h, r) { // two passes of a running-sum box blur each way, close to a Gaussian
    var a = new Float32Array(src), b = new Float32Array(src.length), pass, x, y, acc, k = 1 / (2 * r + 1);
    for (pass = 0; pass < 2; pass++) {
      for (y = 0; y < h; y++) {
        var row = y * w; acc = 0;
        for (x = -r; x <= r; x++) acc += a[row + clamp(x, 0, w - 1)];
        for (x = 0; x < w; x++) { b[row + x] = acc * k; acc += a[row + Math.min(w - 1, x + r + 1)] - a[row + Math.max(0, x - r)]; }
      }
      for (x = 0; x < w; x++) {
        acc = 0;
        for (y = -r; y <= r; y++) acc += b[clamp(y, 0, h - 1) * w + x];
        for (y = 0; y < h; y++) { a[y * w + x] = acc * k; acc += b[Math.min(h - 1, y + r + 1) * w + x] - b[Math.max(0, y - r) * w + x]; }
      }
    }
    return a;
  }
  function shadeRim(w, h, isRound, mat) {
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var ctx = cv.getContext('2d'), img = ctx.createImageData(w, h), d = img.data;
    mat.isRound = isRound;
    var size = Math.min(w, h), rw = mat.width * size, cx = w / 2, cy = h / 2, R = size / 2, n = w * h;
    PXF = 1 / size;
    var H = new Float32Array(n), U = new Float32Array(n), Aa = new Float32Array(n), NX = new Float32Array(n), NY = new Float32Array(n);
    var x, y, i, reach = rw * (mat.reach || 1);
    for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
      i = y * w + x;
      var px = x + 0.5, py = y + 0.5, dist, nx, ny, along;
      if (isRound) {
        var dx = px - cx, dy = py - cy, rr = Math.hypot(dx, dy) || 1;
        dist = R - rr; nx = -dx / rr; ny = -dy / rr; along = (Math.atan2(dy, dx) + Math.PI) * R;
      } else {
        var l = px, ri = w - px, t = py, b = h - py, m = Math.min(l, ri, t, b);
        dist = m;
        if (m === l) { nx = 1; ny = 0; along = py; } else if (m === ri) { nx = -1; ny = 0; along = py + 2 * h; }
        else if (m === t) { nx = 0; ny = 1; along = px + h; } else { nx = 0; ny = -1; along = px + 3 * h; }
      }
      U[i] = dist / rw; Aa[i] = along / size; NX[i] = nx; NY[i] = ny;
      if (dist < 0 || dist > reach) { H[i] = NaN; continue; }
      H[i] = mat.height(dist / rw, (px - cx) / size, (py - cy) / size, along / size, dist / size);
    }
    var depth = (mat.depth || 0.6) * rw, s = {};
    // Ambient occlusion: how far each point sits below the average of its surroundings at three scales, so pits,
    // cracks and the foot of every step darken the way they do under an overcast sky. Missing material counts as low ground.
    var OCC = null, EXPO = null;
    if (mat.ao) {
      var G0 = new Float32Array(n);
      for (i = 0; i < n; i++) G0[i] = H[i] === H[i] ? H[i] : -0.3;
      OCC = new Float32Array(n); EXPO = new Float32Array(n);
      [[0.005, 0.5], [0.014, 0.32], [0.035, 0.18]].forEach(function (lv) {
        var B = boxBlur(G0, w, h, Math.max(1, Math.round(size * lv[0])));
        for (var k = 0; k < n; k++) { var dv = B[k] - G0[k]; if (dv > 0) OCC[k] += dv * lv[1]; else EXPO[k] -= dv * lv[1]; }
      });
    }
    // Self-shadowing: march from each pixel toward the light; if the surface rises above the ray, it is in shadow.
    // The penumbra widens with distance, so cracks and chips get soft, believable shadows.
    var SH = null;
    if (mat.selfShadow) {
      SH = new Float32Array(n);
      var lx = LIGHT[0], ly = LIGHT[1], lxy = Math.hypot(lx, ly), sx = lx / lxy, sy = ly / lxy, rise = LIGHT[2] / lxy;
      var maxT = Math.min(size * 0.2, depth * 2.2 / rise);
      for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
        i = y * w + x; var h0 = H[i];
        if (!(h0 === h0)) continue;
        var z0 = h0 * depth, lit = 1;
        for (var tt = 1.5; tt < maxT; tt += 1.5) {
          var qx = Math.round(x + sx * tt), qy = Math.round(y + sy * tt);
          if (qx < 0 || qy < 0 || qx >= w || qy >= h) break;
          var hq = H[qy * w + qx]; if (!(hq === hq)) continue;
          var over = hq * depth - (z0 + tt * rise);
          if (over > 0) { lit = Math.min(lit, 1 - clamp(over / (tt * 0.35 + 1), 0, 1)); if (lit <= 0) break; }
        }
        SH[i] = lit;
      }
      // The march samples whole pixels, so its shadow edges come out stepped; a pixel's blur makes them clean.
      for (i = 0; i < n; i++) if (!(H[i] === H[i])) SH[i] = 1;
      SH = boxBlur(SH, w, h, 1);
    }
    for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
      i = y * w + x; var o = i * 4, u = U[i];
      if (u < 0) continue;
      var hv = H[i];
      if (u > 1 && !(hv === hv)) { // inside, not covered: the rim's soft shadow on the picture
        var sh = Math.exp(-(u - 1) * rw / (size * 0.0175)) * (mat.shadow == null ? 0.62 : mat.shadow);
        if (sh > 0.004) d[o + 3] = Math.round(sh * 255);
        continue;
      }
      if (!(hv === hv)) continue;
      var hl = x > 0 ? H[i - 1] : hv, hr = x < w - 1 ? H[i + 1] : hv, hu = y > 0 ? H[i - w] : hv, hd = y < h - 1 ? H[i + w] : hv;
      var edge = !(hl === hl) || !(hr === hr) || !(hu === hu) || !(hd === hd);
      if (!(hl === hl)) hl = hv; if (!(hr === hr)) hr = hv; if (!(hu === hu)) hu = hv; if (!(hd === hd)) hd = hv;
      var gx = (hr - hl) * 0.5 * depth, gy = (hd - hu) * 0.5 * depth, nl = Math.hypot(gx, gy, 1);
      s.N = [-gx / nl, -gy / nl, 1 / nl]; s.u = u; s.h = hv; s.A = Aa[i]; s.X = (x + 0.5 - cx) / size; s.Y = (y + 0.5 - cy) / size;
      s.r = u * rw / size; s.nx = NX[i]; s.ny = NY[i]; s.lit = SH ? SH[i] : 1; s.diff = diffuse(s.N) * s.lit; s.px = x; s.py = y;
      s.occ = OCC ? OCC[i] : 0; s.expo = EXPO ? EXPO[i] : 0;
      s.curv = (hl + hr + hu + hd - 4 * hv) * depth; // > 0 in hollows, < 0 on ridges
      if (edge && s.curv > 0) s.curv = 0;           // a broken edge is not a hollow
      var c = mat.color(s);
      d[o] = clamp(c[0], 0, 255); d[o + 1] = clamp(c[1], 0, 255); d[o + 2] = clamp(c[2], 0, 255);
      var a = c.length > 3 ? c[3] : 1;
      if (u * rw < 1.2) a *= u * rw / 1.2;    // soft outer edge
      if (edge) a *= 0.6;                      // soft chipped and torn edges
      d[o + 3] = Math.round(255 * clamp(a, 0, 1));
    }
    ctx.putImageData(img, 0, 0);
    return cv;
  }
  /* ---------- Static materials ---------- */
  var TAU = Math.PI * 2;
  function fract(v) { return v - Math.floor(v); }
  function ridged(x, y, oct) {
    var s = 0, a = 0.5, n = 0;
    for (var i = 0; i < oct; i++) { s += a * (1 - Math.abs(2 * vnoise(x, y) - 1)); n += a; x = x * 2.03 + 17.1; y = y * 2.01 + 9.7; a *= 0.5; }
    return s / n;
  }
  function perOf(mat) { return mat.isRound ? Math.PI : 4; }
  // The point on the outer edge nearest this one, so noise that should vary only round the rim has no seam.
  function edgePt(X, Y, isRound) {
    if (isRound) { var r = Math.hypot(X, Y) || 1; return [X / r * 0.5, Y / r * 0.5]; }
    var m = Math.max(Math.abs(X), Math.abs(Y)) || 1; return [X / m * 0.5, Y / m * 0.5];
  }
  // Distance along the rim, wrapped so it never jumps at the seam.
  function wrapD(d, per) { d = d % per; if (d > per / 2) d -= per; if (d < -per / 2) d += per; return d; }

  // Obsidian: a ring of volcanic glass. The face is ground to a soft dome and polished until it mirrors the room;
  // the outer edge is left as it was knapped, a run of shallow flake scars that meet in sharp ridges.
  var OB = { j: 0, d: 0, da: 0, r: 1 };
  function obsScar(A, u, per, width) { // the lowest flake scar over this point; each is a shallow bowl struck from the edge
    var n = Math.round(per / 0.034), step = per / n, j0 = Math.round(A / step), best = 9, bj = 0, bd = 0, bda = 0, br = 1;
    for (var j = j0 - 2; j <= j0 + 2; j++) {
      var jj = ((j % n) + n) % n, at = (j + 0.4 * (lat(jj, 3) - 0.5)) * step;
      var r = 0.03 + 0.032 * lat(jj, 5), z = 0.04 + 0.16 * lat(jj, 7);
      var da = wrapD(A - at, per), db = u * width + 0.006, dist2 = da * da + db * db;
      var hs = z + dist2 / (r * r) * (0.86 - z);
      if (hs < best) { best = hs; bj = jj; bd = Math.sqrt(dist2); bda = da; br = r; }
    }
    OB.j = bj; OB.d = bd; OB.da = bda; OB.r = br; return best;
  }
  function obsPolish(u) { return (0.5 + 0.42 * Math.pow(Math.sin(Math.PI * clamp(u, 0, 1)), 0.7)) * (1 - 0.25 * smooth(0.88, 1, u)); }
  function obsidianMat(inside) { // inside(s): what lies under the polish, as a colour lit by the room, or null
  return {
    width: 0.1, depth: 0.75, selfShadow: true, shadow: 0.72,
    height: function (u, X, Y, A) {
      var per = perOf(this), sc = obsScar(A, u, per, this.width), p = obsPolish(u);
      var q = OB.da / OB.r;
      if (u < 0.13 * (0.4 + 0.6 * lat(OB.j, 11)) * Math.max(0, 1 - q * q)) return NaN;             // each scar bites into the outline
      if (sc >= p) return p;
      return sc + 0.0012 * Math.sin(OB.d * 900) * smooth(0.004, 0.012, OB.d) * bl(900 / TAU);    // faint conchoidal ripples
    },
    color: function (s) {
      var N = s.N, Rx = 2 * N[2] * N[0], Ry = 2 * N[2] * N[1], Rz = 2 * N[2] * N[2] - 1;
      // The room it mirrors: a window overhead and to the left, a pale ceiling, a dark floor.
      var kx = Rx + 0.3, ky = Ry + 0.36, wx = kx * 0.8 + ky * 0.6, wy = -kx * 0.6 + ky * 0.8;
      var win = (1 - smooth(0.13, 0.16, Math.abs(wx))) * (1 - smooth(0.2, 0.23, Math.abs(wy))) * 120 + Math.exp(-(kx * kx + ky * ky) / 0.12) * 34;
      var fill = Math.pow(Math.max(0, -Rx * 0.62 - Ry * 0.5 + Rz * 0.6), 6) * 34;              // a dim second light, low right
      var key = Math.pow(Math.max(0, Rx * LIGHT[0] + Ry * LIGHT[1] + Rz * LIGHT[2]), 40) * 90;
      var sky = 22 * smooth(-0.6, 0.6, -Ry) + 10 * smooth(0.3, 1, Rz), F = 0.06 + 0.94 * Math.pow(1 - N[2], 4);
      var e = (win + key) * s.lit * (0.55 + F) + (sky + fill) * (0.4 + F);
      var ridge = smooth(0.25, 1.2, -s.curv) * 24 * (0.4 + 0.6 * s.lit);
      // Under the polish, faint bands of rainbow sheen where the glass flowed as it cooled.
      var flow = s.X * 3 + s.Y * 9 + 1.6 * fbm(s.X * 3 + 4, s.Y * 3, 3), band = 0.5 + 0.5 * Math.sin(flow * 9);
      var sheen = smooth(0.55, 0.95, band) * smooth(0.45, 0.75, fbm(s.X * 5, s.Y * 5 + 2, 2)) * 16;
      var hue = flow * 2.2, sr = 0.5 + 0.5 * Math.sin(hue), sg = 0.5 + 0.5 * Math.sin(hue + 2.1), sb = 0.5 + 0.5 * Math.sin(hue + 4.2);
      var smoky = smooth(0.12, 0.02, s.u) * 18;                        // thin knapped edges let light through, smoky brown
      var b = [7, 7, 9];
      if (inside) { var c = inside(s), lit = 0.25 + 0.75 * s.diff; b = mix3(b, c.map(function (v) { return v * lit; }), c[3]); sheen *= 1 - c[3] * 0.7; }
      return [b[0] + e + ridge + sheen * sr + smoky * 1.2, b[1] + e + ridge + sheen * sg + smoky * 0.9, b[2] + e * 1.03 + ridge + sheen * sb + smoky * 0.7];
    }
  };
  }
  var OBSIDIAN = obsidianMat(null);
  // Snowflake obsidian: grey-white crystals grown in the glass, soft blotches made of fine radiating needles.
  var SNOWFLAKE = obsidianMat(function (s) {
    var v = voronoi(s.X * 11 + 3, s.Y * 11);
    if (v.id > 0.8) return [0, 0, 0, 0];
    var dx = s.X * 11 + 3 - v.cx, dy = s.Y * 11 - v.cy, ang = Math.atan2(dy, dx), R = 0.2 + 0.24 * lat(Math.floor(v.cx * 5) & 255, 9);
    var edge = 0.65 + 0.5 * fbm(Math.cos(ang) * 1.5 + v.id * 40, Math.sin(ang) * 1.5, 3);
    var rr = v.f1 / (R * edge), needles = 0.6 + 0.4 * vnoise(ang * 9 + v.id * 30, v.f1 * 4);
    var m = (1 - smooth(0.25, 1, rr)) * needles * (0.6 + 0.4 * smooth(0.3, 0.7, fbm(dx * 12, dy * 12, 2)));
    return [200, 200, 196, Math.min(1, m * 1.1)];
  });
  // Mahogany obsidian: red-brown iron oxide swirled through the black as the glass flowed.
  var MAHOGANY = obsidianMat(function (s) {
    var w = fbm(s.X * 3 + 5, s.Y * 3, 3), f = fbm(s.X * 5 + w * 2.4, s.Y * 5 - w * 1.8, 4);
    var m = smooth(0.48, 0.6, f) * (0.7 + 0.3 * smooth(0.4, 0.7, fbm(s.X * 14, s.Y * 14 + 4, 2)));
    return mix3([96, 34, 18], [140, 58, 30], fbm(s.X * 9, s.Y * 9, 2)).concat([m * 0.85]);
  });
  /* ---------- Stone ----------
     A ring cut by a mason: a broad band with a rounded outer edge, a groove, and a rounded bead next to the
     picture. Real carved stone reads as stone because its faces are calm and its edges tell the story, so the
     faces carry only fine grain and the wear goes where wear really goes: the arrises are rounded back unevenly,
     a few flakes have broken off them in flat facets, and a hairline crack or two runs across. The polished
     kinds use the same carving with a mirror finish instead of weathering. */
  // The mason's profile across the rim, 0 at the outer edge to 1 at the picture.
  function stoneBand(u) { return 0.97 - 0.05 * Math.pow((u - 0.4) / 0.27, 2); }   // the band, very slightly domed
  function stoneProfile(u) {
    if (u < 0.13) { var t = 1 - u / 0.13; return 0.14 + (stoneBand(0.13) - 0.14) * Math.sqrt(Math.max(0, 1 - t * t)); }   // rounded outer arris
    if (u < 0.64) return stoneBand(u);
    if (u < 0.7) { var q = (u - 0.64) / 0.06; return 0.58 + (stoneBand(0.64) - 0.58) * Math.sqrt(Math.max(0, 1 - q * q)); }  // rolled into the groove
    if (u < 0.74) return 0.58 - 0.03 * Math.sin(Math.PI * (u - 0.7) / 0.04);                      // the groove floor
    if (u < 0.95) { var b = (u - 0.845) / 0.105; return 0.58 + 0.3 * Math.sqrt(Math.max(0, 1 - b * b)); }  // the bead
    return 0.58 - 0.16 * smooth(0.95, 1, u);                                                       // a small fillet down to the picture
  }
  // How exposed an arris is at u: wear and chips live on edges, not on faces.
  function stoneEdgeness(u) {
    return Math.max(1 - smooth(0.02, 0.17, u), Math.exp(-Math.pow((u - 0.66) / 0.035, 2)), Math.exp(-Math.pow((u - 0.845) / 0.06, 2)) * 0.7);
  }
  // Flakes broken off an edge: where round the rim (fraction), the edge (0 outer, 0.95 the bead's inner side), length
  // and depth as fractions of the rim width. Each break is two flat fracture faces meeting in a ridge.
  var STONE_CHIPS = [[0.03, 0, 1.0, 0.24], [0.17, 0, 0.5, 0.14], [0.29, 0.95, 0.6, 0.12], [0.41, 0, 1.4, 0.28], [0.55, 0.95, 0.5, 0.1],
    [0.66, 0, 0.6, 0.16], [0.78, 0, 0.8, 0.2], [0.93, 0, 0.45, 0.12]];
  var STONE_CRACKS = [[0.12, 0.7], [0.47, 1.0], [0.81, 0.55]]; // where, and how far in from the outer edge it runs
  var SF = { chip: 0, crack: 0, stain: 0 };
  function stoneFeatures(m, A, u, X, Y) {
    var per = perOf(m), wdt = m.width, seed = m.kind.seed, H = 9, fresh = 0, crack = 0, stain = 0;
    if (m.kind.chips) for (var j = 0; j < STONE_CHIPS.length; j++) {
      var c = STONE_CHIPS[j], at = fract(c[0] + seed * 0.073) * per, len = c[2] * wdt * 0.5, dep = c[3] * wdt;
      var da = wrapD(A - at, per);
      if (Math.abs(da) > len * 1.3) continue;
      var dv = Math.abs(u - c[1]) * wdt;                                                 // in from the edge it broke from
      var q = Math.hypot(da / len, dv / dep) * (1 + 0.3 * (vnoise(da * 160 + j * 7, dv * 160) - 0.5) + 0.05 * (vnoise(da * 400, dv * 400 + j) - 0.5) * bl(400));
      if (q > 1) continue;
      // A spall: a shallow fracture face sloping down to the broken edge, in two facets that meet in a ridge,
      // ending at the back in a low step where the flake came away.
      var x = dv / dep, tilt = (lat(j, 7) - 0.5) * 0.3 / len, b0 = c[1] ? 0.5 : 0.08;
      var p1 = b0 + 0.78 * x + tilt * da, p2 = b0 + 0.1 + 0.6 * x - tilt * da * 0.8;
      var pl = Math.min(p1, p2) + 0.012 * (vnoise(da * 500 + j, dv * 500) - 0.5) + 0.015 * Math.sin(x * 11 + j) * x;   // faint conchoidal ripples
      pl = lerp(pl, 1.3, smooth(0.72, 1, q));                                          // the break dies out at its rim, no cliff
      if (pl < H) H = pl;
      fresh = Math.max(fresh, 1 - smooth(0.82, 1, q));
    }
    if (m.kind.cracks) for (var k = 0; k < STONE_CRACKS.length; k++) {
      var ck = STONE_CRACKS[k];
      if (u > ck[1]) continue;
      var path = fract(ck[0] + seed * 0.111) * per + 0.018 * (fbm(u * 3 + k * 5 + seed, k, 3) - 0.5) + 0.004 * (vnoise(u * 20 + k * 3, 9) - 0.5);
      var dd = Math.abs(wrapD(A - path, per)), fade = 1 - smooth(ck[1] - 0.25, ck[1], u);
      var w = 0.0016 * (0.6 + 0.6 * vnoise(u * 14 + k * 9, 3)) * fade;
      if (w > 0) { crack = Math.max(crack, clamp(1 - dd / w, 0, 1)); stain = Math.max(stain, (1 - smooth(0, 0.012, dd)) * fade); }
    }
    SF.chip = H; SF.fresh = fresh; SF.crack = crack; SF.stain = stain; return SF;
  }
  function stoneHeight(m, u, X, Y, A) {
    var K = m.kind;
    // The carving is by hand: the profile drifts a little round the ring.
    var uu = u + 0.012 * (fbm(A * 9 + K.seed, 3, 3) - 0.5);
    var h = stoneProfile(clamp(uu, 0, 1));
    if (K.wear) {
      // Edges worn back unevenly: broad nibbles out of the arrises, none on the faces.
      var ed = stoneEdgeness(uu), nib = fbm(X * 22 + K.seed, Y * 22, 4), nib2 = vnoise(X * 70 + 4, Y * 70);
      h -= ed * K.wear * (0.1 * smooth(0.35, 0.75, nib) + 0.035 * nib2);
      if (u < 0.045 * smooth(0.5, 0.75, fbm(A * 14 + K.seed, 7, 3))) return NaN;    // the outline is worn back in places
      h -= (fbm(X * 5 + 3, Y * 5, 2) - 0.5) * 0.04 * K.wear;                         // the band itself is worn very slightly hollow
    }
    // Fine texture: grain and the odd pore, small enough to read as surface, not as lumps.
    h += (vnoise(X * 260, Y * 260) - 0.5) * 0.012 * K.grain * bl(260);
    if (K.pores) { var pv = voronoi(X * 150 + 7, Y * 150); if (pv.id < K.pores) h -= (1 - smooth(0.04, 0.22, pv.f1)) * 0.04 * bl(150); }
    var f = stoneFeatures(m, A, u, X, Y);
    if (f.chip < h + 0.03) {                                                      // a smooth minimum: a crisp step, never a jagged one
      var k = 0.03, m2 = clamp(0.5 + 0.5 * (h - f.chip) / k, 0, 1); h = lerp(h, f.chip, m2) - k * m2 * (1 - m2);
      if (h < 0.03) return NaN;
    }
    return h - f.crack * 0.35;
  }
  function stoneColor(m, s) {
    var K = m.kind, X = s.X, Y = s.Y, f = stoneFeatures(m, s.A, s.u, X, Y);
    var alb = K.base(X, Y, s);
    // Mineral grains, one per pixel of the master: a few dark, a few pale. Scaled down, they become the stone's tooth.
    var g = hash(s.px * 1.37 + K.seed, s.py * 0.91), g2 = hash(s.px * 0.73 + 11, s.py * 1.19 + K.seed);
    var gk = g < K.dark ? 0.72 : g > 1 - K.pale ? 1.12 : 0.96 + 0.08 * g2;
    alb = alb.map(function (v) { return v * gk; });
    if (f.fresh > 0) alb = mix3(alb, K.fresh(X, Y), f.fresh * 0.8);              // a fresh break is cleaner than the skin
    var occ = clamp(s.occ * 3.5, 0, 1) * (1 - f.fresh * 0.7), expo = clamp(s.expo * 5, 0, 1);   // a fresh break is open, not a hollow
    if (K.grime) alb = mix3(alb, K.grime, clamp(occ * 0.5 + f.stain * 0.35, 0, 0.8) * (1 - f.fresh * 0.7));   // dirt in the hollows and along cracks
    if (K.algae) {                                                                     // a green-grey film in the damp of the groove
      var al = smooth(0.45, 0.7, fbm(X * 8 + 30, Y * 8, 3)) * clamp(occ * 1.6, 0, 1) * K.algae;
      alb = mix3(alb, [96, 104, 70], al * 0.55);
    }
    if (K.wear) alb = alb.map(function (v) { return v * (1 + expo * 0.12); });      // worn edges are paler
    // Matte stone scatters light: a wrapped key light, a soft sky from above, both dimmed in the hollows.
    var NL = s.N[0] * LIGHT[0] + s.N[1] * LIGHT[1] + s.N[2] * LIGHT[2], wrap = clamp((NL + 0.25) / 1.25, 0, 1) * s.lit;
    var sky = 0.55 + 0.45 * s.N[2], ao = 1 - occ * 0.6;
    var key = wrap * (K.keyK || 0.95), amb = sky * (K.ambK || 0.38) * ao;
    var out = [alb[0] * (key * 1.02 + amb * 0.92), alb[1] * (key + amb * 0.97), alb[2] * (key * 0.94 + amb * 1.06)];
    if (K.polish) {                                                                    // a mirror finish shows the room
      var env = stonePolish(s, K.polish);
      out = [out[0] * (1 - env.f) + env.c, out[1] * (1 - env.f) + env.c, out[2] * (1 - env.f) + env.c * 1.03];
    } else {
      var sp = specular(s.N, 10) * 0.025 * s.lit * ao;
      out = out.map(function (v) { return v + 255 * sp; });
    }
    if (f.crack > 0) out = out.map(function (v) { return v * (1 - f.crack * 0.55); });
    return out;
  }
  // Reflection of the room in polished stone: a window up and to the left, a pale ceiling, and Fresnel, so the
  // reflection is faint on the flat band and strong on the rolled edges and the bead. Shared with obsidian's room.
  function stonePolish(s, amt) {
    var N = s.N, Rx = 2 * N[2] * N[0], Ry = 2 * N[2] * N[1], Rz = 2 * N[2] * N[2] - 1;
    var kx = Rx + 0.3, ky = Ry + 0.36, wx = kx * 0.8 + ky * 0.6, wy = -kx * 0.6 + ky * 0.8;
    var win = (1 - smooth(0.13, 0.16, Math.abs(wx))) * (1 - smooth(0.2, 0.23, Math.abs(wy))) * 130 + Math.exp(-(kx * kx + ky * ky) / 0.12) * 40;
    var key = Math.pow(Math.max(0, Rx * LIGHT[0] + Ry * LIGHT[1] + Rz * LIGHT[2]), 60) * 160;
    var sky = 60 * smooth(-0.6, 0.6, -Ry) + 40 * smooth(0.3, 1, Rz);
    var F = (0.05 + 0.95 * Math.pow(1 - N[2], 4)) * amt;
    return { f: F * 0.5, c: (win * s.lit + key * s.lit + sky) * F };
  }
  function stoneMat(kind) {
    return { width: 0.12, depth: 0.85, shadow: 0.72, selfShadow: true, ao: true, kind: kind,
      height: function (u, X, Y, A) { return stoneHeight(this, u, X, Y, A); },
      color: function (s) { return stoneColor(this, s); } };
  }
  // Limestone: pale and warm, weathered grey on the band in soft patches, with a few dark pores.
  var SANDSTONE = stoneMat({ seed: 0, wear: 1, grain: 1, pores: 0.05, chips: true, cracks: true, dark: 0.05, pale: 0.12, algae: 0.6,
    grime: [88, 80, 66], fresh: function () { return [206, 196, 174]; },
    base: function (X, Y) {
      var tone = fbm(X * 3 + 2, Y * 3, 3), alb = mix3([172, 162, 142], [194, 184, 162], tone);
      alb = mix3(alb, [186, 160, 122], smooth(0.5, 0.8, fbm(X * 4 + 20, Y * 4, 3)) * 0.35);          // warm iron staining
      alb = mix3(alb, [134, 132, 126], smooth(0.55, 0.75, fbm(X * 6 + 9, Y * 6 + 4, 4)) * 0.4);       // weathered grey crust
      return alb.map(function (v) { return v * (0.95 + 0.1 * fbm(X * 18, Y * 18, 3)); });
    } });
  // Polished granite, crystals at three sizes the way the rock grew: big blocky feldspar, mostly salmon pink with
  // some milky white; grey glassy quartz filling between them; and the dark mica in small flakes that gather in
  // clots and streaks, never spread evenly, and are brown-black rather than black. An even scatter of separate black
  // grains on pink reads as insects, not stone.
  function graniteBase(X, Y) {
    var wx = 0.35 * fbm(X * 40, Y * 40, 2), wy = 0.35 * fbm(X * 40 + 5, Y * 40, 2);
    var v = voronoi(X * 64 + wx, Y * 64 + wy), fid = v.id, fseam = v.f2 - v.f1, fx = v.cx, fy = v.cy;
    var tone = lat(Math.floor(fx * 3 + 40) & 255, Math.floor(fy * 3) & 255), drift = smooth(0.3, 0.7, fbm(X * 6 + 3, Y * 6, 3));
    var c = fid < 0.22 ? mix3([226, 220, 210], [206, 200, 192], tone)                                  // white plagioclase
      : mix3(mix3([208, 150, 130], [228, 178, 158], tone), [190, 134, 118], drift * 0.45);               // pink feldspar
    c = mix3(c, c.map(function (q) { return q * 0.9; }), smooth(0.4, 0.8, vnoise(X * 260 + fid * 50, Y * 90)));   // cleavage sheen inside a crystal
    var q = voronoi(X * 150 + wx, Y * 150 + wy), qid = q.id, qseam = q.f2 - q.f1;
    if (qid < 0.16 + 0.08 * drift) {                                                                    // quartz, grey and glassy: the crystal under it shows through
      c = mix3(c, mix3([128, 128, 134], [160, 158, 160], 0.4 * smooth(0, 0.6, q.f1) + 0.6 * lat(Math.floor(q.cx * 5) & 255, 9)), 0.75);
      fseam = qseam;
    }
    var clot = smooth(0.56, 0.8, fbm(X * 26 + 21, Y * 26 - 4, 3));                                      // where the mica gathers
    var mv = voronoi(X * 330 + wx * 2, Y * 330 + wy * 2);
    if (mv.id < clot * 0.75 + 0.03) {
      var flake = lat(Math.floor(mv.cx * 3) & 255, Math.floor(mv.cy * 3 + 7) & 255);
      c = mix3([40, 34, 32], [70, 62, 56], flake);                                                     // biotite, brown-black
      if (flake > 0.9) c = mix3(c, [150, 140, 120], 0.5);                                              // the odd flake catching the light
      fseam = Math.min(fseam, mv.f2 - mv.f1);
    }
    c = mix3(c, [128, 116, 110], (1 - smooth(0, 0.06, fseam)) * 0.25);                                 // grains meet in a fine seam
    var inner = 0.94 + 0.12 * vnoise(X * 700, Y * 700) * bl(700);
    return c.map(function (q) { return q * inner * (0.95 + 0.08 * fbm(X * 3 + 9, Y * 3, 3)); });
  }
  var GRANITE = stoneMat({ seed: 3, wear: 0, grain: 0.3, pores: 0, chips: true, cracks: false, dark: 0.02, pale: 0.03, polish: 1, keyK: 0.8, ambK: 0.32,
    fresh: function (X, Y) { return graniteBase(X, Y).map(function (v) { return v * 1.08; }); }, base: graniteBase });
  // Polished marble: white with soft grey clouds and veins. The veins are healed fractures, so they run as a
  // branching network, sharp where narrow and smoky where they widen.
  function marbleBase(X, Y) {
    // Veins are healed fractures: they stream one broad way, bent by a slow warp, branch into finer veins, and
    // fade in and out. Ridged noise gives the thin bright crest of each; it is darkened, not drawn.
    var wx = X + 0.3 * fbm(X * 2 + 4, Y * 2, 4), wy = Y + 0.3 * fbm(X * 2, Y * 2 + 8, 4);
    var along = wx * 0.55 + wy * 0.84, across = -wx * 0.84 + wy * 0.55;
    var n1 = fbm(across * 2.6 + 0.8 * fbm(along * 3, across * 3 + 3, 3), along * 0.9 + 4, 5), r1 = 1 - Math.abs(2 * n1 - 1);
    var n2 = fbm(across * 6 + 1.2 * fbm(along * 6 + 2, across * 6, 3), along * 2.2 + 9, 4), r2 = 1 - Math.abs(2 * n2 - 1);
    var n3 = fbm(across * 14 + 2 * fbm(along * 10, across * 10 + 5, 2), along * 5, 3), r3 = 1 - Math.abs(2 * n3 - 1);
    var keep1 = smooth(0.38, 0.62, fbm(along * 2 + 1, across * 2 + 6, 3)), keep2 = smooth(0.45, 0.7, fbm(along * 4 + 5, across * 4, 3));
    var vein = Math.pow(r1, 26) * keep1 * 0.85 + Math.pow(r2, 34) * keep2 * 0.5 + Math.pow(r3, 40) * keep2 * 0.3;
    var haze = Math.pow(r1, 5) * keep1 * 0.25;
    var cloud = smooth(0.42, 0.78, fbm(X * 4 + 2, Y * 4 + marbleWarp(X, Y), 4));
    var alb = mix3([241, 239, 234], [229, 225, 216], fbm(X * 3.5 + 7, Y * 3.5, 3));
    alb = mix3(alb, [212, 212, 212], cloud * 0.35);
    alb = mix3(alb, [196, 196, 200], haze);
    return mix3(alb, [112, 114, 122], Math.min(0.9, vein));
  }
  function marbleWarp(X, Y) { return 1.4 * fbm(X * 2.5 + 4, Y * 2.5, 3); }
  var MARBLE = stoneMat({ seed: 6, wear: 0, grain: 0.2, pores: 0, chips: true, cracks: false, dark: 0.01, pale: 0.02, polish: 0.85, keyK: 0.72, ambK: 0.44,
    fresh: function (X, Y) { return marbleBase(X, Y).map(function (v) { return Math.min(255, v * 1.04); }); }, base: marbleBase });
  /* ---------- Weathered metal ----------
     Two cast beads with a groove between them, dented and pitted with age. Each metal ages its own way:
     bronze goes green and powdery in the recesses, silver tarnishes black, iron rusts. The high points that
     hands and use have rubbed stay bright. */
  function twinBeads(u) { return 0.75 * bead(0.03, 0.46, u) + 0.6 * bead(0.54, 0.97, u); }
  function metalMat(M) {
    return {
      width: 0.085, depth: 0.6, shadow: 0.62, ao: true, metal: M,
      height: function (u, X, Y) {
        var h = twinBeads(u) + (fbm(X * 9 + 40, Y * 9, 3) - 0.5) * 0.14;                    // dents: old metal is never a true profile
        h += (vnoise(X * 60, Y * 60) - 0.5) * 0.03 * M.rough;
        if (M.flake) h += (fbm(X * 44 + 9, Y * 44, 3) - 0.5) * 0.1 * M.flake + (vnoise(X * 130, Y * 130) - 0.5) * 0.04 * M.flake * bl(130);   // rust lifts in flakes
        var pv = voronoi(X * 70 + 3, Y * 70);
        if (pv.id < M.pits) h -= (1 - smooth(0.05, 0.28, pv.f1)) * 0.07;                      // pits where corrosion ate in
        return h;
      },
      color: function (s) {
        var X = s.X, Y = s.Y, hh = clamp(s.h, 0, 1.2), grain = vnoise(s.A * 300, s.r * 30) * bl(300);
        var occ = clamp(s.occ * 5, 0, 1), diff = s.diff;
        var spec = specular(s.N, M.shine) * 0.75 * s.lit;
        var lum = (0.16 + diff * 1.05) * (0.55 + 0.45 * Math.min(1, hh)) * (1 - occ * 0.4) * (0.92 + grain * 0.16);
        var out = [M.base[0] * lum + M.spec[0] * spec, M.base[1] * lum + M.spec[1] * spec, M.base[2] * lum + M.spec[2] * spec];
        // Patina: matte and powdery, thickest in the recesses and in drifting patches.
        var pn = fbm(X * 7 + 3, Y * 7, 3) * 0.55 + fbm(X * 24, Y * 24, 2) * 0.3 + vnoise(X * 90, Y * 90) * 0.15 * bl(90);
        var vg = smooth(0.52, 0.86, pn * 0.8 + (1 - Math.min(1, hh)) * 0.5 + occ * 0.3 + M.bias) * M.patinaAmt;
        var high = smooth(0.62, 0.9, hh) * (1 - vg) * 0.7;                                      // rubbed bright on the high points
        var vl = (0.32 + diff * 0.7) * (0.85 + grain * 0.3) * (0.8 + pn * 0.4) * (M.flake ? 0.75 + 0.5 * vnoise(X * 110, Y * 110) * bl(110) : 1);
        var pc = M.patina2 ? mix3(M.patina, M.patina2, fbm(X * 30 + 5, Y * 30, 3)) : M.patina;                // rust and verdigris are never one flat colour
        for (var k = 0; k < 3; k++) out[k] = (out[k] * (1 + high * 0.35) + M.spec[k] * spec * high * 0.8) * (1 - vg) + pc[k] * vl * vg;
        var grime = smooth(0.5, 0.9, 1 - Math.min(1, hh)) * 0.5 + occ * 0.3;                    // grime in the deepest part
        for (k = 0; k < 3; k++) out[k] = out[k] * (1 - grime) + M.grime[k] * grime;
        var sc = vnoise(s.A * 60 + s.r * 14 + 11, s.r * 60 - s.A * 10);                           // a few bright scratches
        if (Math.abs(sc - 0.5) < 0.005 && vg < 0.3) { out[0] += 38; out[1] += 32; out[2] += 26; }
        return out;
      }
    };
  }
  var BRONZE = metalMat({ base: [140, 92, 56], spec: [255, 210, 165], shine: 30, rough: 1, pits: 0.06, patina: [108, 146, 124], patina2: [84, 128, 104], patinaAmt: 0.92, bias: 0.06, grime: [52, 40, 28] });
  var SILVER = metalMat({ base: [186, 188, 192], spec: [255, 255, 255], shine: 44, rough: 0.6, pits: 0.02, patina: [58, 50, 44], patinaAmt: 0.8, bias: -0.12, grime: [40, 34, 30] });
  var RUSTIRON = metalMat({ base: [78, 76, 74], spec: [200, 200, 205], shine: 22, rough: 1.4, pits: 0.12, patina: [156, 80, 36], patina2: [86, 40, 20], patinaAmt: 1, bias: 0.08, flake: 1, grime: [64, 34, 18] });
  // Adventurer's leather: a strap pieced together from different hides over years on the road. Each patch is
  // lapped over the last and saddle-stitched down with waxed thread; one seam is laced with a thong. No metal.
  var HIDES = [ // colour, surface: 0 smooth, 1 pebbled, 2 suede
    [[124, 78, 42], 0], [[156, 108, 62], 1], [[92, 42, 30], 0], [[78, 72, 44], 2], [[170, 126, 80], 0], [[100, 62, 36], 1], [[64, 46, 32], 0]
  ];
  var LACED = 3; // the seam that is laced instead of stitched
  var PAT = { j: 0, d: 0 };
  // Which hide a point is on, and how far past that hide's own cut edge it is (the newer hide laps over the older).
  function patchAt(A, u, per) {
    var t = A / per, n = HIDES.length, jj = n - 1, dj = 9;
    for (var j = 0; j < n; j++) {
      var b = (j + 0.5 * (lat(j, 3) - 0.5)) / n + 0.03 * (u - 0.5) * (lat(j, 5) * 2 - 1) + 0.004 * Math.sin(u * 5 + j * 2);
      var dd = wrapD((t - b) * per, per);
      if (dd >= 0 && dd < dj) { dj = dd; jj = j; }
    }
    PAT.j = jj; PAT.d = dj; return PAT;
  }
  var LS = { thread: 0, chan: 0, hole: 0, lace: 0 };
  function leatherStitch(A, u, d, j, width) {
    var thread = 0, chan = 0, hole = 0, lace = 0, v = u * width;
    if (j === LACED) {                    // a thong laced in an X through punched holes
      var P = 0.013, ph = fract(v / P + 0.25), x1 = (ph - 0.5) * 0.016 + 0.009, x2 = -(ph - 0.5) * 0.016 + 0.009;
      var l1 = 1 - Math.abs(d - x1) / 0.0021, l2 = 1 - Math.abs(d - x2) / 0.0021;
      lace = Math.max(0, ph < 0.5 ? l2 * 0.92 : l2, ph < 0.5 ? l1 : l1 * 0.92);
      var hp = Math.hypot((fract(v / P + 0.25) < 0.5 ? fract(v / P + 0.25) : 1 - fract(v / P + 0.25)) * P, Math.min(Math.abs(d - 0.001), Math.abs(d - 0.017)));
      if (hp < 0.0019) hole = 1 - hp / 0.0019;
    } else if (d < 0.012) {               // saddle stitches running beside the lapped edge
      var dc = d - 0.0055, phs = fract(v / 0.0058 + dc * 60);
      chan = Math.max(0, 1 - Math.abs(dc) / 0.0016);
      if (Math.abs(dc) < 0.0011 && phs < 0.68) thread = bead(-0.0011, 0.0011, dc) * bead(0, 0.68, phs);
    }
    // Stitching along both edges of the strap.
    [0.0115, width - 0.0115].forEach(function (e0) {
      var de = v - e0;
      if (Math.abs(de) < 0.0016) {
        chan = Math.max(chan, 1 - Math.abs(de) / 0.0016);
        var pe = fract(A / 0.0062 + de * 70);
        if (Math.abs(de) < 0.001 && pe < 0.66) thread = Math.max(thread, bead(-0.001, 0.001, de) * bead(0, 0.66, pe));
      }
    });
    LS.thread = thread; LS.chan = chan; LS.hole = hole; LS.lace = lace; return LS;
  }
  function leatherMat(hides, lace) {
  return {
    width: 0.115, depth: 0.5, selfShadow: true, shadow: 0.66, hides: hides,
    height: function (u, X, Y, A) {
      var per = perOf(this), p = patchAt(A, u, per), d = p.d, kind = this.hides[p.j][1];
      var h = 0.5 * smooth(0, 0.08, u) * (1 - smooth(0.92, 1, u)) + 0.08 * bead(0.03, 0.97, u);
      h += 0.09 * smooth(0, 0.0012, d) * (1 - smooth(0.014, 0.026, d));   // the patch's cut edge, lapped over the hide below
      if (kind === 1) { var pv = voronoi(X * 260 + p.j * 7, Y * 260); h += 0.02 * (1 - pv.f1 * pv.f1) * bl(260); }
      h += 0.02 * (ridged(A * 22 + p.j * 3, u * 2, 3) - 0.5) + 0.012 * (vnoise(X * 120, Y * 120) - 0.5) * bl(120);
      var st = leatherStitch(A, u, d, p.j, this.width);
      return h - st.chan * 0.025 + st.thread * 0.05 + st.lace * 0.075 - st.hole * 0.05;
    },
    color: function (s) {
      var per = perOf(this), p = patchAt(s.A, s.u, per), j = p.j, kind = this.hides[j][1], X = s.X, Y = s.Y;
      var st = leatherStitch(s.A, s.u, p.d, j, this.width);
      if (st.lace > 0.05) {              // the thong: darker leather, glossy where it is worn
        var ll = 0.18 + s.diff * 0.95, lsp = specular(s.N, 24) * 0.25 * s.lit;
        return [lace[0] * ll + 255 * lsp, lace[1] * ll + 235 * lsp, lace[2] * ll + 200 * lsp];
      }
      if (st.thread > 0.05) {            // waxed linen thread
        var tl = (0.2 + s.diff * 0.9) * (0.75 + 0.25 * st.thread), tsp = specular(s.N, 30) * 0.2 * s.lit;
        return [206 * tl + 255 * tsp, 190 * tl + 250 * tsp, 152 * tl + 230 * tsp];
      }
      var alb = this.hides[j][0].slice(), m = fbm(X * 9 + j * 5, Y * 9 + j, 3);
      alb = mix3(alb, alb.map(function (v) { return v * 0.7; }), smooth(0.3, 0.8, m) * 0.6);
      alb = mix3(alb, [40, 26, 16], smooth(0.8, 1, Math.abs(s.u - 0.5) * 2) * 0.55);        // grime and oil at the edges
      alb = mix3(alb, [42, 26, 16], (1 - smooth(0, 0.0016, p.d)) * 0.8);                    // the cut edge, burnished dark
      var hi = smooth(0.62, 0.8, fbm(X * 14 + 30, Y * 14, 3)) * smooth(0.3, 0.9, s.h);          // scuffed pale on the high spots
      if (kind !== 2) alb = mix3(alb, alb.map(function (v) { return Math.min(255, v * 1.35 + 18); }), hi * 0.5);
      var sc = fbm(X * 10 + j, Y * 10, 2), scr = (1 - smooth(0.001, 0.0035, Math.abs(fbm(X * 16 + sc, Y * 16 - sc, 3) - 0.5))) * smooth(0.62, 0.7, fbm(X * 5 + 11, Y * 5, 2));
      if (kind !== 2) alb = mix3(alb, alb.map(function (v) { return Math.min(255, v * 1.5 + 30); }), scr * 0.6);  // a few scratches
      alb = mix3(alb, alb.map(function (v) { return v * 0.75; }), st.chan * 0.6);              // the stitch channel is pressed darker
      if (st.hole > 0) alb = mix3(alb, [20, 12, 8], st.hole);
      var ao = clamp(1 - Math.max(0, s.curv) * 0.6, 0.5, 1);
      var fuzz = kind === 2 ? 0.9 + 0.2 * vnoise(X * 300, Y * 300) * bl(300) : 1;
      var lum = (0.16 + s.diff * 0.95 + 0.05 * s.N[2]) * ao * fuzz;
      var sp = kind === 2 ? 0 : specular(s.N, kind ? 10 : 16) * (kind ? 0.08 : 0.16) * s.lit;
      return [alb[0] * lum + 255 * sp, alb[1] * lum + 235 * sp, alb[2] * lum + 210 * sp];
    }
  };
  }
  var LEATHER = leatherMat(HIDES, [62, 40, 26]);
  // Ranger's gear: greens and browns, more suede, the kind of leather that hides in a wood.
  var RANGER = leatherMat([[[74, 82, 46], 0], [[110, 84, 52], 1], [[58, 66, 40], 2], [[132, 98, 60], 0], [[86, 96, 58], 2], [[96, 64, 38], 1], [[52, 58, 36], 0]], [44, 36, 24]);
  // Oxblood: deep reds and near-blacks, dyed leather gone dark with oil.
  var OXBLOOD = leatherMat([[[92, 30, 26], 0], [[54, 26, 24], 1], [[118, 44, 34], 0], [[40, 30, 28], 2], [[132, 56, 40], 0], [[70, 30, 26], 1], [[30, 24, 22], 0]], [26, 20, 18]);
  var IRON = {
    width: 0.06, depth: 0.6,
    height: function (u, X, Y, A, r) { return 0.75 * bead(0.03, 0.46, u) + 0.6 * bead(0.54, 0.97, u) + 0.03 * fbm(A * 90, r * 90, 2); },
    color: function (s) {
      var g = fbm(s.A * 120, s.r * 12, 3), lum = (0.16 + s.diff * 1.0) * (0.55 + 0.45 * clamp(s.h, 0, 1)) * (0.9 + g * 0.2);
      var sp = specular(s.N, 24) * 0.55;
      return [62 * lum + 200 * sp, 60 * lum + 200 * sp, 58 * lum + 205 * sp];
    }
  };

  /* ---------- Ink brush (enso) on a washi paper disc, with a red seal ---------- */
  // Papers: base colour and how much darker the fibres run.
  var PAPERS = { washi: [238, 229, 206], black: [34, 32, 36], kraft: [200, 164, 118], night: [36, 42, 64] };
  var paperTex = {};
  function paperTexture(kind) {
    kind = kind || 'washi';
    if (paperTex[kind]) return paperTex[kind];
    var P0 = PAPERS[kind], dk = kind === 'black' || kind === 'night' ? 0.35 : 1;
    var c = document.createElement('canvas'); c.width = c.height = 256;
    var g = c.getContext('2d'), im = g.createImageData(256, 256), d = im.data;
    for (var y = 0; y < 256; y++) for (var x = 0; x < 256; x++) {
      var o = (y * 256 + x) * 4, n = fbm(x / 22, y / 22, 4) * 0.6 + fbm(x / 3, y / 60, 2) * 0.25 + hash(x, y) * 0.15;
      d[o] = P0[0] - n * 26 * dk; d[o + 1] = P0[1] - n * 28 * dk; d[o + 2] = P0[2] - n * 30 * dk; d[o + 3] = 255;
    }
    g.putImageData(im, 0, 0); paperTex[kind] = c; return c;
  }
  // ink: the colour of the ink; halo: a faint dark bleed under light ink so it still reads on a pale map.
  function inkLayer(w, h, ink, halo) {
    ink = ink || [14, 14, 20];
    var key = 'ink|' + w + '|' + h + '|' + ink.join(',') + '|' + !!halo;
    if (layerCache.has(key)) return layerCache.get(key);
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), S0 = Math.min(w, h), cx = w / 2, cy = h / 2, R0 = S0 * 0.4;
    var bw = S0 * 0.085;
    g.save(); g.lineCap = 'round';
    // A soft bleed under the stroke, where wet ink spread into the paper.
    g.filter = 'blur(' + Math.max(1, S0 * 0.006) + 'px)';
    g.strokeStyle = rgba(ink, 0.16); g.lineWidth = bw * 1.1;
    g.beginPath(); g.arc(cx, cy, R0, -2.1, -2.1 + 5.75 * 0.55); g.stroke();
    if (halo) { g.filter = 'blur(' + Math.max(1, S0 * 0.012) + 'px)'; g.strokeStyle = 'rgba(0,0,0,.4)'; g.lineWidth = bw * 1.3; g.beginPath(); g.arc(cx, cy, R0, -2.1, -2.1 + 5.75 * 0.9); g.stroke(); }
    g.filter = 'none';
    var start = -2.1, sweep = 5.75, bristles = 90, steps = 220;
    function wbw(t) { return bw * smooth(0, 0.03, t) * (0.6 + 0.5 * Math.exp(-t * 6) - 0.45 * t) * (1 + 0.08 * Math.sin(t * 9)); }
    // The wet body of the stroke: solid ink that runs thin and breaks up as the brush dries.
    for (var q = 0; q < steps; q++) {
      var t0 = q / steps, t1 = (q + 1) / steps, wet = 0.88 * (1 - smooth(0.35, 0.85, t0));
      if (wet < 0.02) break;
      var a0 = start + sweep * t0, a1 = start + sweep * t1, w0 = wbw(t0) * 0.42, w1 = wbw(t1) * 0.42;
      g.fillStyle = rgba(ink, wet); g.beginPath();
      g.moveTo(cx + Math.cos(a0) * (R0 - w0), cy + Math.sin(a0) * (R0 - w0)); g.lineTo(cx + Math.cos(a0) * (R0 + w0), cy + Math.sin(a0) * (R0 + w0));
      g.lineTo(cx + Math.cos(a1) * (R0 + w1), cy + Math.sin(a1) * (R0 + w1)); g.lineTo(cx + Math.cos(a1) * (R0 - w1), cy + Math.sin(a1) * (R0 - w1)); g.fill();
    }
    for (var b = 0; b < bristles; b++) {
      var off = (b / (bristles - 1) - 0.5), lw = Math.max(0.6, S0 * 0.0022) * (0.6 + hash(b, 3) * 0.9);
      var prev = null;
      for (var k = 0; k <= steps; k++) {
        var t = k / steps, a = start + sweep * t;
        var width = wbw(t);
        var rr = R0 + off * width + S0 * 0.0012 * Math.sin(t * 5 + b);
        var pt = [cx + Math.cos(a) * rr, cy + Math.sin(a) * rr];
        var dry = vnoise(b * 1.7, t * 26) < smooth(0.35, 1, t) * 0.75 + Math.abs(off) * 0.5 * smooth(0.2, 1, t);
        if (prev && !dry && Math.abs(off) < 0.5 * (1.05 - 0.2 * t)) {
          g.strokeStyle = rgba(ink.map(function (v) { return v * (0.8 + 0.4 * hash(b, 11)); }), (0.35 + 0.4 * hash(b, 9)) * (1 - 0.3 * t)); g.lineWidth = lw;
          g.beginPath(); g.moveTo(prev[0], prev[1]); g.lineTo(pt[0], pt[1]); g.stroke();
        }
        prev = pt;
      }
    }
    g.restore();
    layerCache.set(key, cv);
    return cv;
  }
  // The seal: carved vermilion, its stamp texture uneven where the paste didn't take. It is its own layer so
  // recolouring the ink leaves the seal red.
  function sealLayer(w, h) {
    var key = 'seal|' + w + '|' + h;
    if (layerCache.has(key)) return layerCache.get(key);
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), S0 = Math.min(w, h), cx = w / 2, cy = h / 2, R0 = S0 * 0.4;
    var sz = S0 * 0.11, sx = cx + R0 * 0.78, sy = cy + R0 * 0.72;
    var sc = document.createElement('canvas'); sc.width = sc.height = Math.max(8, Math.round(sz));
    var s2 = sc.getContext('2d'), n = sc.width;
    s2.fillStyle = '#b3291f'; s2.fillRect(0, 0, n, n);
    s2.strokeStyle = '#f3e8d3'; s2.lineWidth = Math.max(1, n * 0.08); s2.lineCap = 'square';
    s2.beginPath(); s2.moveTo(n * 0.3, n * 0.22); s2.lineTo(n * 0.3, n * 0.78); s2.moveTo(n * 0.3, n * 0.5); s2.lineTo(n * 0.7, n * 0.5);
    s2.moveTo(n * 0.7, n * 0.22); s2.lineTo(n * 0.7, n * 0.78); s2.moveTo(n * 0.45, n * 0.3); s2.lineTo(n * 0.58, n * 0.3); s2.stroke();
    var im = s2.getImageData(0, 0, n, n), d = im.data;
    for (var i = 0; i < d.length; i += 4) { var px = (i / 4) % n, py = Math.floor(i / 4 / n); d[i + 3] = 255 * smooth(0.25, 0.5, fbm(px / n * 9, py / n * 9, 3)) * (0.75 + 0.25 * hash(px, py)); }
    s2.putImageData(im, 0, 0);
    g.save(); g.translate(sx, sy); g.rotate(-0.06); g.globalAlpha = 0.92; g.drawImage(sc, -sz / 2, -sz / 2, sz, sz); g.restore();
    layerCache.set(key, cv);
    return cv;
  }
  function washi(ctx, w, h, kind) {
    var S0 = Math.min(w, h), cx = w / 2, cy = h / 2;
    ctx.save(); ctx.beginPath();
    for (var k = 0; k <= 180; k++) {
      var a = k / 180 * TAU, rr = S0 * (0.485 - 0.012 * fbm(Math.cos(a) * 3 + 5, Math.sin(a) * 3 + 5, 3));
      if (k) ctx.lineTo(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr); else ctx.moveTo(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr);
    }
    ctx.closePath(); ctx.shadowColor = 'rgba(0,0,0,.35)'; ctx.shadowBlur = S0 * 0.02; ctx.shadowOffsetY = S0 * 0.008;
    ctx.fillStyle = ctx.createPattern(paperTexture(kind), 'repeat'); ctx.fill(); ctx.restore();
  }

  /* ---------- Live effects ----------
     Smoke and flame are computed once per frame into a small texture in token space and scaled into every
     canvas that shows them, so ten previews cost the same as one. Noise here uses a lookup table, not sin. */
  function qn(x, y) {
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi;
    var X0 = xi & 255, Y0 = yi & 255, X1 = (X0 + 1) & 255, Y1 = (Y0 + 1) & 255;
    var a = HT[Y0 * 256 + X0], b = HT[Y0 * 256 + X1], c = HT[Y1 * 256 + X0], d = HT[Y1 * 256 + X1];
    var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
    return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v;
  }
  function qfbm(x, y, oct) { var s = 0, a = 0.5, n = 0; for (var i = 0; i < oct; i++) { s += a * qn(x, y); n += a; x = x * 2.03 + 17.1; y = y * 2.01 + 9.7; a *= 0.5; } return s / n; }

  var fxCache = {};
  function fxTexture(kind, t, fn, N) {
    var q = Math.round(t * 30), c = fxCache[kind];
    if (c && c.q === q) return c.cv;
    if (!c) { c = fxCache[kind] = { cv: document.createElement('canvas') }; c.cv.width = c.cv.height = N; c.img = c.cv.getContext('2d').createImageData(N, N); }
    var d = c.img.data;
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, o = (y * N + x) * 4;
      fn(X, Y, t, d, o);
    }
    c.cv.getContext('2d').putImageData(c.img, 0, 0); c.q = q;
    return c.cv;
  }

  // Living shadow: the ring is the smoke itself. Domain-warped smoke rolls round the ring in two layers at
  // different speeds. It is thickest along the ring, thins into wisps that curl out over the map and in over the
  // picture's edge, and a little cold light shows through the thinner wisps.
  // The smoke's density and shading are worked out once per frame; each kind of smoke only colours them.
  var SHADOW_R = 0.43, SHADOW_N = 256;
  var shadowField = { q: -1, dens: new Float32Array(SHADOW_N * SHADOW_N), bil: new Float32Array(SHADOW_N * SHADOW_N), thin: new Float32Array(SHADOW_N * SHADOW_N) };
  function shadowDensity(t) {
    var q = Math.round(t * 30), F = shadowField, N = SHADOW_N;
    if (F.q === q) return F;
    var ca = Math.cos(t * 0.07), sa = Math.sin(t * 0.07);
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, i = y * N + x, r = Math.hypot(X, Y), dr = r - SHADOW_R;
      F.dens[i] = 0;
      if (dr < -0.14 || dr > 0.07) continue;
      var RX = X * ca - Y * sa, RY = X * sa + Y * ca;
      var q1 = qfbm(RX * 3.5 + t * 0.1, RY * 3.5 - t * 0.15, 2), q2 = qfbm(RX * 3.5 + 5.2 - t * 0.08, RY * 3.5 + 1.3 + t * 0.12, 2);
      var n = qfbm(RX * 8 + q1 * 3.4 - t * 0.2, RY * 8 + q2 * 3.4 + t * 0.3, 4);
      var reachOut = 0.045 + 0.035 * q1, reachIn = 0.05 + 0.13 * smooth(0.38, 0.7, q2);
      var env = dr > 0 ? 1 - smooth(0, reachOut, dr) : 1 - smooth(0, reachIn, -dr);
      var core = Math.exp(-Math.pow(dr / 0.03, 2)), nf = qfbm(RX * 16 + q2 * 4 + t * 0.35, RY * 16 - q1 * 4 - t * 0.2, 2);
      // The envelope raises the bar the noise has to clear, so the smoke breaks into distinct wisps as it spreads.
      var dens = Math.max(core * 0.95, smooth(0.44, 0.58, n + 0.42 * env - 0.24) * 0.97, smooth(0.5, 0.62, nf + 0.3 * env - 0.2) * 0.8 * env) * (1 - smooth(0.058, 0.07, dr));
      if (dens <= 0.003) continue;
      // Billows inside the thick smoke catch a little light, and the thin wisps more.
      F.dens[i] = dens;
      F.bil[i] = smooth(0.48, 0.78, qfbm(RX * 12 - q2 * 2 + t * 0.15, RY * 12 + q1 * 2 - t * 0.25, 2));
      F.thin[i] = (1 - dens) * smooth(0.35, 0.7, q1 + n * 0.5);
    }
    F.q = q; return F;
  }
  // Each kind: the colour of the thick smoke, of the lit billows, of the thin wisps, and how its density shows.
  var SMOKES = {
    shadow: { base: [5, 6, 9], bil: [30, 30, 30], thin: [58, 58, 58], pow: 0.7, alpha: 1 }
  };
  var smokeTex = {};
  function shadowTexture(kind, t) {
    var F = shadowDensity(t), tk = kind + '#' + gradeId(), c = smokeTex[tk], K0 = SMOKES[kind], N = SHADOW_N;
    if (c && c.q === F.q) return c.cv;
    if (!c) { c = smokeTex[tk] = { cv: document.createElement('canvas') }; c.cv.width = c.cv.height = N; c.img = c.cv.getContext('2d').createImageData(N, N); }
    var K = { base: tc(K0.base), bil: tc(K0.bil), thin: tc(K0.thin), pow: K0.pow, alpha: K0.alpha };
    var d = c.img.data;
    for (var i = 0; i < N * N; i++) {
      var o = i * 4, dens = F.dens[i];
      if (dens <= 0) { d[o + 3] = 0; continue; }
      var b = F.bil[i] * dens, th = F.thin[i];
      d[o] = K.base[0] + K.bil[0] * b + K.thin[0] * th; d[o + 1] = K.base[1] + K.bil[1] * b + K.thin[1] * th; d[o + 2] = K.base[2] + K.bil[2] * b + K.thin[2] * th;
      d[o + 3] = Math.round(255 * Math.pow(dens, K.pow) * K.alpha);
    }
    c.cv.getContext('2d').putImageData(c.img, 0, 0); c.q = F.q;
    return c.cv;
  }

  // Fire: seen from above, the ring burns all the way round and every flame streams outward from it, away over
  // the map and a shorter way in over the picture's edge. The flame shapes are noise carried outward along the
  // flames and swayed sideways more the further they reach, so tongues lick, split and curl as they go.
  // One intensity field is worked out per frame and coloured for each kind of fire.
  var FIRE_R = 0.395, FIRE_N = 320, FIRE_A = 64;
  var fireField = { q: -1, v: new Float32Array(FIRE_N * FIRE_N), sm: new Float32Array(FIRE_N * FIRE_N), h: new Float32Array(FIRE_N * FIRE_N), px: null };
  // Where each pixel sits round the ring and how far out from it, worked out once.
  function firePixels() {
    var list = [], N = FIRE_N;
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, r = Math.hypot(X, Y), dr = r - FIRE_R;
      if (dr < -0.07 || dr > 0.104) continue;
      list.push(y * N + x, (Math.atan2(Y, X) / TAU + 0.5) * FIRE_A, dr, r);
    }
    return new Float32Array(list);
  }
  function fireIntensity(t) {
    var q = Math.round(t * 30), F = fireField;
    if (F.q === q) return F;
    if (!F.px) F.px = firePixels();
    var V = F.v, SM = F.sm, Hh = F.h, PX = F.px;
    V.fill(0); SM.fill(0);
    for (var k = 0; k < PX.length; k += 4) {
      var i = PX[k], a = PX[k + 1], dr = PX[k + 2], r = PX[k + 3];
      var out = dr >= 0, s = out ? dr : -dr, Hf = out ? 0.088 : 0.042, side = out ? 0 : 40;
      var tongue = pvn(a * 0.25 + side, t * 0.4, FIRE_A / 4);                          // some stretches burn higher for a while
      var Hk = Hf * (0.45 + 1.0 * tongue), h01 = s / Hk;
      var warp = (pfbm(a * 0.5 + 3 + side, s * 9 - t * 1.4, FIRE_A / 2, 2) - 0.5) * 2.4 * Math.min(1.4, 0.25 + h01);
      var n = pfbm(a + warp, s * 20 - t * 2.8 + side, FIRE_A, 3);
      var n2 = pvn(a * 2 + warp * 2 + 5, s * 46 - t * 5 + side, FIRE_A * 2);              // finer licks inside each tongue
      var v = (1 - h01) * 0.8 + (n - 0.5) * 2.1 + (n2 - 0.5) * 0.45 * smooth(0.1, 0.6, h01) - 0.12;
      v = Math.max(v, Math.exp(-dr * dr / 0.0002) * (0.52 + 0.7 * (n - 0.5)));     // the bed of the fire along the ring never goes out
      v *= 1 - smooth(0.468, 0.497, r);
      V[i] = v; Hh[i] = h01;
      if (out && v <= 0 && h01 > 0.55 && h01 < 2.2) {                               // thin smoke off the tips
        SM[i] = smooth(0.55, 1, h01) * (1 - smooth(1.4, 2.2, h01)) * smooth(0.5, 0.75, pfbm(a * 0.5 + warp * 0.5 + 17, s * 8 - t * 0.9, FIRE_A / 2, 2)) * 0.22 * (1 - smooth(0.46, 0.497, r));
      }
    }
    F.q = q; return F;
  }
  var FIRES = {
    orange: { ramp: [[0, [96, 12, 4]], [0.22, [186, 42, 8]], [0.45, [242, 112, 22]], [0.72, [255, 186, 66]], [1, [255, 242, 196]]], glow: [255, 120, 36], smoke: [46, 40, 36] },
    blue: { ramp: [[0, [14, 22, 96]], [0.22, [26, 66, 206]], [0.45, [48, 150, 255]], [0.72, [136, 216, 255]], [1, [238, 250, 255]]], glow: [70, 150, 255], smoke: [30, 36, 50] },
    green: { ramp: [[0, [12, 58, 14]], [0.22, [30, 138, 24]], [0.45, [92, 216, 44]], [0.72, [190, 255, 96]], [1, [246, 255, 214]]], glow: [110, 230, 70], smoke: [30, 42, 30] }
  };
  function rampAt(ramp, v) {
    for (var k = 1; k < ramp.length; k++) if (v <= ramp[k][0]) return mix3(ramp[k - 1][1], ramp[k][1], (v - ramp[k - 1][0]) / (ramp[k][0] - ramp[k - 1][0]));
    return ramp[ramp.length - 1][1];
  }
  // The fire's colours under the grade in force.
  var firePals = {};
  function firePal(kind) {
    var k = kind + '#' + gradeId(), p = FIRES[kind];
    if (!firePals[k]) firePals[k] = { ramp: p.ramp.map(function (e) { return [e[0], tc(e[1])]; }), glow: tc(p.glow), smoke: tc(p.smoke) };
    return firePals[k];
  }
  var fireTex = {};
  function fireTexture(kind, t) {
    var F = fireIntensity(t), tk = kind + '#' + gradeId(), c = fireTex[tk], pal = firePal(kind), N = FIRE_N;
    if (c && c.q === F.q) return c.cv;
    if (!c) { c = fireTex[tk] = { cv: document.createElement('canvas') }; c.cv.width = c.cv.height = N; c.img = c.cv.getContext('2d').createImageData(N, N); }
    var d = c.img.data, V = F.v, SM = F.sm;
    for (var i = 0; i < N * N; i++) {
      var v = V[i], o = i * 4;
      if (v > 0) {
        var cc = rampAt(pal.ramp, Math.min(1, v));
        d[o] = cc[0]; d[o + 1] = cc[1]; d[o + 2] = cc[2]; d[o + 3] = Math.round(255 * smooth(0, 0.3, v) * (1 - 0.45 * smooth(0.5, 1.2, F.h[i])));   // tips thin to transparent
      } else if (SM[i] > 0.004) { d[o] = pal.smoke[0]; d[o + 1] = pal.smoke[1]; d[o + 2] = pal.smoke[2]; d[o + 3] = Math.round(255 * SM[i]); }
      else d[o + 3] = 0;
    }
    c.cv.getContext('2d').putImageData(c.img, 0, 0); c.q = F.q;
    return c.cv;
  }
  function fireFront(kind) {
    return function (ctx, G, t) {
      var tex = fireTexture(kind, t), W = G.outW, H = G.outH, pal = firePal(kind);
      fireGlow(ctx, G, t, pal.glow);
      ctx.save(); ctx.imageSmoothingQuality = 'high'; ctx.drawImage(tex, 0, 0, W, H);
      ctx.globalCompositeOperation = 'lighter'; ctx.globalAlpha = 0.15;                // the flames light the air around them
      ctx.drawImage(fireHaze(kind, tex), 0, 0, W, H);
      ctx.restore();
      fireAsh(ctx, G, t, kind);
      fireEmbers(ctx, G, t, kind);
    };
  }
  // The glow round the flames: the flame texture shrunk to a few dozen pixels, which blurs it for free when it is drawn back up.
  var hazeTex = {};
  function fireHaze(kind, tex) {
    var hk = kind + '#' + gradeId(), h = hazeTex[hk];
    if (!h) { h = hazeTex[hk] = { cv: document.createElement('canvas'), q: -2 }; h.cv.width = h.cv.height = 48; }
    if (h.q !== fireField.q) { var g = h.cv.getContext('2d'); g.clearRect(0, 0, 48, 48); g.imageSmoothingQuality = 'high'; g.drawImage(tex, 0, 0, 48, 48); h.q = fireField.q; }
    return h.cv;
  }
  // Embers: specks of burning fuel thrown off the bed. Each is a tiny white-hot point with a soft glow and a short
  // streak along its path; it is flung out fast, slows, wanders on the hot air, and cools through orange to dull red.
  var emberSprites = {};
  function emberSprite(kind) {
    var ek = kind + '#' + gradeId();
    if (emberSprites[ek]) return emberSprites[ek];
    var c = document.createElement('canvas'); c.width = c.height = 32;
    var g = c.getContext('2d'), gl = firePal(kind).glow, gr = g.createRadialGradient(16, 16, 0, 16, 16, 16);
    gr.addColorStop(0, rgba(gl, 0.55)); gr.addColorStop(0.35, rgba(gl, 0.16)); gr.addColorStop(1, rgba(gl, 0));
    g.fillStyle = gr; g.fillRect(0, 0, 32, 32); emberSprites[ek] = c; return c;
  }
  function fireEmbers(ctx, G, t, kind) {
    var T = G.outW, cx = T / 2, cy = G.outH / 2, ramp = firePal(kind).ramp, spr = emberSprite(kind);
    ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.lineCap = 'round';
    for (var i = 0; i < 70; i++) {
      var life = 0.6 + 1.2 * hash(i, 2), cyc = t / life + hash(i, 1), ph = fract(cyc), id = i * 131 + Math.floor(cyc) * 7;
      var a0 = hash(id, 3) * TAU, inward = hash(id, 8) < 0.18, reach = T * (inward ? 0.03 + 0.05 * hash(id, 4) : 0.05 + 0.12 * hash(id, 4));
      var pos = function (p) {
        var e = 1 - Math.pow(1 - p, 1.8), rr = T * FIRE_R + (inward ? -1 : 1) * reach * e;
        var aa = a0 + (qn(i * 1.7 + 3, p * 2.5 + t * 0.3) - 0.5) * 0.5 * p * (T * 0.4 / rr);
        return [cx + Math.cos(aa) * rr, cy + Math.sin(aa) * rr];
      };
      var p1 = pos(ph), p0 = pos(Math.max(0, ph - 0.02 - 0.06 * hash(id, 7))), temp = Math.pow(1 - ph, 0.8) * (0.75 + 0.25 * hash(id, 5));
      var flick = 0.65 + 0.35 * qn(i * 3.1, t * 14), alpha = Math.min(1, ph * 12) * flick * smooth(0, 0.25, temp);
      if (alpha <= 0.01) continue;
      var col = rampAt(ramp, 0.35 + 0.65 * temp), sz = T * (0.0016 + 0.0016 * hash(id, 6)) * (0.6 + 0.4 * temp);
      ctx.globalAlpha = alpha * 0.9; ctx.drawImage(spr, p1[0] - sz * 7, p1[1] - sz * 7, sz * 14, sz * 14);
      ctx.globalAlpha = alpha * 0.7; ctx.strokeStyle = rgba(col, 1); ctx.lineWidth = Math.max(0.6, sz * 2.2);
      ctx.beginPath(); ctx.moveTo(p0[0], p0[1]); ctx.lineTo(p1[0], p1[1]); ctx.stroke();
      ctx.globalAlpha = alpha; ctx.strokeStyle = rgba(rampAt(ramp, Math.min(1, 0.6 + 0.6 * temp)), 1); ctx.lineWidth = Math.max(0.4, sz);   // the white-hot core
      ctx.beginPath(); ctx.moveTo((p0[0] + p1[0] * 2) / 3, (p0[1] + p1[1] * 2) / 3); ctx.lineTo(p1[0], p1[1]); ctx.stroke();
    }
    ctx.restore();
  }
  // Ash: flakes of burnt-out fuel, thin, curled and ragged, with holes burnt through. Most are charcoal grey, some
  // have gone pale, and fresh ones still glow along their edges. They lift off slowly and turn over as they drift.
  var ASH = null;
  function ashSprites() {
    if (ASH) return ASH;
    ASH = [];
    for (var k = 0; k < 16; k++) {
      var N = 48, body = document.createElement('canvas'), glow = document.createElement('canvas');
      body.width = body.height = glow.width = glow.height = N;
      var bg = body.getContext('2d'), gg = glow.getContext('2d'), bi = bg.createImageData(N, N), gi = gg.createImageData(N, N);
      var kind = k % 4, base = [44, 58, 118, 176][kind], el = 0.45 + 0.4 * hash(k, 9), ca = hash(k, 4) * TAU, cs = Math.cos(ca), sn = Math.sin(ca);
      for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
        var X = (x + 0.5) / N * 2 - 1, Y = ((y + 0.5) / N * 2 - 1) / el, rr = Math.hypot(X, Y), o = (y * N + x) * 4;
        // A lobed, torn outline: broad noise minus distance, with a fine ragged fringe.
        var f = fbm(X * 2.4 + k * 7.3, Y * 2.4 + k * 3.1, 3) - rr * 0.62 + 0.08 * (vnoise(X * 11 + k, Y * 11) - 0.5);
        var ins = f - 0.12, holes = fbm(X * 5 + k * 5, Y * 5 + 2, 2);
        var m = smooth(0, 0.035, ins) * (1 - smooth(0.6, 0.66, holes));
        if (m <= 0) continue;
        var curl = 0.72 + 0.5 * (X * cs + Y * sn) * 0.5;                                      // a curled sheet, lit on one side
        var tone = base * curl * (0.85 + 0.3 * fbm(X * 6 + k, Y * 6 + 9, 2)) + (kind < 2 ? 28 : -36) * (1 - smooth(0, 0.07, ins));
        bi.data[o] = tone * 1.02; bi.data[o + 1] = tone; bi.data[o + 2] = tone * 0.95; bi.data[o + 3] = 255 * m * (kind === 3 ? 0.8 : 0.9);
        var band = (1 - smooth(0, 0.06, Math.min(ins, 0.6 - holes))) * m;
        gi.data[o] = gi.data[o + 1] = gi.data[o + 2] = 255; gi.data[o + 3] = 255 * band;
      }
      bg.putImageData(bi, 0, 0); gg.putImageData(gi, 0, 0);
      ASH.push({ body: body, glow: glow });
    }
    return ASH;
  }
  var ashTint = document.createElement('canvas');
  function fireAsh(ctx, G, t, kind) {
    var T = G.outW, cx = T / 2, cy = G.outH / 2, sp = ashSprites(), gl = firePal(kind).glow;
    ctx.save();
    for (var i = 0; i < 26; i++) {
      var life = 2.6 + 2.4 * hash(i, 12), cyc = t / life + hash(i, 11), ph = fract(cyc), id = i * 97 + Math.floor(cyc) * 13;
      var a0 = hash(id, 13) * TAU, rr = T * (FIRE_R + 0.01 + (0.03 + 0.07 * hash(id, 14)) * ph);
      var aa = a0 + Math.sin(t * (0.8 + hash(id, 15)) + i) * 0.04 * ph + (qn(i * 1.3, ph * 2 + 5) - 0.5) * 0.3 * ph;
      var x = cx + Math.cos(aa) * rr, y = cy + Math.sin(aa) * rr;
      var fade = smooth(0, 0.12, ph) * (1 - smooth(0.7, 1, ph)), s = sp[id % sp.length], sz = T * (0.006 + 0.008 * hash(id, 17));
      var tumble = 0.2 + 0.8 * Math.abs(Math.cos(t * (0.7 + 0.9 * hash(id, 19)) + i * 2)), spin = t * (0.4 + 1.2 * hash(id, 18)) + i;
      ctx.save(); ctx.translate(x, y); ctx.rotate(spin); ctx.scale(1, tumble);
      ctx.globalAlpha = fade; ctx.drawImage(s.body, -sz, -sz, sz * 2, sz * 2);
      var hot = hash(id, 21) < 0.45 ? 1 - smooth(0.05, 0.45, ph) : 0;               // fresh flakes still burning at the edges
      if (hot > 0.02) { ctx.globalCompositeOperation = 'lighter'; ctx.globalAlpha = fade * hot; ctx.drawImage(tintedGlow(s.glow, gl), -sz, -sz, sz * 2, sz * 2); }
      ctx.restore();
    }
    ctx.restore();
  }
  var glowTints = new Map();
  function tintedGlow(src, col) {
    var key = src; var m = glowTints.get(key) || {}; var k2 = col.join(',');
    if (m[k2]) return m[k2];
    var c = document.createElement('canvas'); c.width = src.width; c.height = src.height;
    var g = c.getContext('2d'); g.drawImage(src, 0, 0); g.globalCompositeOperation = 'source-in';
    g.fillStyle = rgba(col.map(function (v) { return Math.min(255, v + 40); }), 1); g.fillRect(0, 0, c.width, c.height);
    m[k2] = c; glowTints.set(key, m); return c;
  }
  // Firelight on the edge of the picture and on the map round the ring, flickering with the flames.
  function fireGlow(ctx, G, t, col) {
    var cx = G.outW / 2, cy = G.outH / 2, R = G.outW * FIRE_R, f = 0.16 + 0.07 * (qn(t * 3, 1) - 0.5) * 2;
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    var g = ctx.createRadialGradient(cx, cy, R * 0.72, cx, cy, G.outW * 0.5);
    g.addColorStop(0, rgba(col, 0)); g.addColorStop(0.55, rgba(col, f)); g.addColorStop(1, rgba(col, 0));
    ctx.fillStyle = g; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
  }

  /* ---------- Arcane: runes in blue light, lightning arcing round the band, and three orbiting crystals ---------- */
  var RUNES = (function () {
    var out = [];
    for (var i = 0; i < 26; i++) {
      var strokes = [], n = 2 + Math.floor(hash(i, 1) * 3);
      for (var k = 0; k < n; k++) {
        var a = Math.floor(hash(i, k * 2 + 3) * 9), b = Math.floor(hash(i, k * 2 + 4) * 9);
        if (a === b) b = (b + 4) % 9;
        strokes.push([[a % 3 - 1, Math.floor(a / 3) - 1], [b % 3 - 1, Math.floor(b / 3) - 1]]);
      }
      strokes.push([[0, -1], [0, 1]]);   // a stave, so each reads as a rune rather than a scribble
      out.push(strokes);
    }
    return out;
  })();
  function hueOf(c) {
    var r = c[0] / 255, g = c[1] / 255, b = c[2] / 255, mx = Math.max(r, g, b), mn = Math.min(r, g, b), d = mx - mn;
    if (!d) return 210;
    var h = mx === r ? ((g - b) / d) % 6 : mx === g ? (b - r) / d + 2 : (r - g) / d + 4;
    return (h * 60 + 360) % 360;
  }
  function satOf(c) { return Math.min(1, (Math.max(c[0], c[1], c[2]) - Math.min(c[0], c[1], c[2])) / 140); }
  function bolt(ctx, pts, rough, seed, depth) { // midpoint displacement between two points
    var out = [pts[0]];
    (function sub(a, b, d, s) {
      if (d === 0) { out.push(b); return; }
      var mx = (a[0] + b[0]) / 2, my = (a[1] + b[1]) / 2, len = Math.hypot(b[0] - a[0], b[1] - a[1]);
      var nx = -(b[1] - a[1]) / len, ny = (b[0] - a[0]) / len, off = (hash(s, d) - 0.5) * len * rough;
      var m = [mx + nx * off, my + ny * off];
      sub(a, m, d - 1, s * 2 + 1); sub(m, b, d - 1, s * 2 + 2);
    })(pts[0], pts[1], depth, seed);
    ctx.beginPath(); out.forEach(function (p, i) { ctx[i ? 'lineTo' : 'moveTo'](p[0], p[1]); });
    return out;
  }
  // A small faceted crystal, lit from the top left: a bright face, a dark face, a glint on the point, and an inner glow.
  function crystal(ctx, x, y, L, spin, hue, glow, sat) {
    var sk = sat == null ? 1 : sat;   // a grey light makes grey crystals
    var w = L * 0.34 * Math.abs(Math.cos(spin)) + L * 0.06, side = Math.cos(spin) > 0 ? 1 : -1;
    var top = [0, -L], ut = [w, -L * 0.35], ub = [w, L * 0.4], bot = [0, L * 0.9], lb = [-w, L * 0.4], lt = [-w, -L * 0.35], mid = [w * 0.15 * side, 0];
    ctx.save(); ctx.translate(x, y); ctx.rotate(Math.sin(spin * 0.5) * 0.3);
    ctx.globalCompositeOperation = 'lighter';
    var halo = ctx.createRadialGradient(0, 0, 0, 0, 0, L * 2.6); halo.addColorStop(0, 'hsla(' + hue + ',' + Math.round(95 * sk) + '%,72%,' + (0.5 * glow).toFixed(3) + ')'); halo.addColorStop(1, 'hsla(' + hue + ',' + Math.round(95 * sk) + '%,65%,0)');
    ctx.fillStyle = halo; ctx.beginPath(); ctx.arc(0, 0, L * 2.6, 0, TAU); ctx.fill();
    ctx.globalCompositeOperation = 'source-over';
    function face(p, col) { ctx.beginPath(); p.forEach(function (q, i) { ctx[i ? 'lineTo' : 'moveTo'](q[0], q[1]); }); ctx.closePath(); ctx.fillStyle = col; ctx.fill(); }
    face([top, ut, ub, bot, mid], 'hsla(' + (hue + 25) + ',' + Math.round(85 * sk) + '%,' + (side > 0 ? 78 : 38) + '%,.96)');
    face([top, lt, lb, bot, mid], 'hsla(' + (hue - 25) + ',' + Math.round(85 * sk) + '%,' + (side > 0 ? 40 : 76) + '%,.96)');
    face([top, mid, ut], 'hsla(' + hue + ',' + Math.round(60 * sk) + '%,92%,.9)');
    face([bot, mid, lb], 'hsla(' + hue + ',' + Math.round(90 * sk) + '%,28%,.85)');
    var core = ctx.createRadialGradient(0, L * 0.05, 0, 0, L * 0.05, L * 0.55); core.addColorStop(0, 'rgba(255,255,255,' + (0.7 * glow).toFixed(3) + ')'); core.addColorStop(1, 'rgba(255,255,255,0)');
    ctx.globalCompositeOperation = 'lighter'; ctx.fillStyle = core; ctx.beginPath(); ctx.arc(0, L * 0.05, L * 0.55, 0, TAU); ctx.fill();
    ctx.globalCompositeOperation = 'source-over';
    ctx.strokeStyle = 'rgba(255,255,255,.6)'; ctx.lineWidth = Math.max(0.5, L * 0.045);
    ctx.beginPath(); [top, ut, ub, bot, lb, lt].forEach(function (q, i) { ctx[i ? 'lineTo' : 'moveTo'](q[0], q[1]); }); ctx.closePath(); ctx.stroke();
    var gl = Math.pow(Math.max(0, Math.sin(spin * 1.3)), 12);                   // a glint as a face turns to the light
    if (gl > 0.02) { ctx.strokeStyle = 'rgba(255,255,255,' + gl.toFixed(3) + ')'; ctx.lineWidth = Math.max(0.5, L * 0.06); ctx.beginPath(); ctx.moveTo(-L * 0.8, -L); ctx.lineTo(L * 0.8, -L); ctx.moveTo(0, -L * 1.8); ctx.lineTo(0, -L * 0.2); ctx.stroke(); }
    ctx.restore();
  }
  function arcBolt(ctx, p1, p2, pm, seed, S0, tint, alpha) {
    var base = tint.map(function (v) { return Math.min(255, v + 120); });
    [[S0 * 0.014, 0.22], [S0 * 0.004, 1]].forEach(function (pass) {
      ctx.shadowColor = rgba(tint, 1); ctx.shadowBlur = S0 * 0.025;
      ctx.strokeStyle = pass[1] < 1 ? rgba(base, alpha * pass[1]) : 'rgba(255,255,255,' + alpha.toFixed(3) + ')';
      ctx.lineWidth = Math.max(0.8, pass[0]);
      bolt(ctx, [p1, pm], 0.5, seed * 7 + 1, 4); ctx.stroke();
      bolt(ctx, [pm, p2], 0.5, seed * 7 + 2, 4); ctx.stroke();
    });
    ctx.shadowBlur = 0;
  }
  // pal.tint: the colour of the light, which the colour control changes; pal.bolts: how often lightning strikes.
  var ARCANES = { blue: { tint: [110, 170, 255], bolts: 1.6 } };
  function arcaneFront(kind) { return function (ctx, G, t) { arcane(ctx, G, t, ARCANES[kind]); }; }
  function arcane(ctx, G, t, pal) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, Ro = S0 * 0.47, Ri = S0 * 0.385, Rm = (Ro + Ri) / 2;
    var tint = tc(pal.tint);
    // A soft halo spilling onto the map.
    ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.lineCap = 'butt';
    for (var hseg = 0; hseg < 24; hseg++) {
      var a0 = hseg / 24 * TAU;
      [[0.05, 0.05], [0.025, 0.09]].forEach(function (pass) {
        ctx.strokeStyle = rgba(tint.map(function (v) { return Math.min(255, v + 60); }), pass[1]);
        ctx.lineWidth = S0 * pass[0]; ctx.beginPath(); ctx.arc(cx, cy, Ro + S0 * pass[0] * 0.4, a0, a0 + TAU / 24 + 0.01); ctx.stroke();
      });
    }
    ctx.restore();
    // The band: deep night, with two prismatic sheens turning against each other.
    ctx.save();
    ctx.beginPath(); ctx.arc(cx, cy, Ro, 0, TAU); ctx.arc(cx, cy, Ri, 0, TAU, true);
    var bg = ctx.createRadialGradient(cx - S0 * 0.1, cy - S0 * 0.12, Ri * 0.8, cx, cy, Ro);
    bg.addColorStop(0, rgba(mix3(tc([38, 42, 60]), tint, 0.12), 1)); bg.addColorStop(1, rgba(mix3(tc([14, 16, 24]), tint, 0.06), 1)); ctx.fillStyle = bg;
    ctx.shadowColor = 'rgba(0,0,0,.5)'; ctx.shadowBlur = S0 * 0.02; ctx.shadowOffsetY = S0 * 0.006; ctx.fill('evenodd');
    ctx.shadowColor = 'transparent'; ctx.globalCompositeOperation = 'lighter';
    [[t * 0.35, 0.06], [-t * 0.22 + 2, 0.04]].forEach(function (cg0) {
      var cg = ctx.createConicGradient(cg0[0], cx, cy);
      for (var h = 0; h <= 6; h++) cg.addColorStop(h / 6, rgba(tint, cg0[1] * 2 * (0.5 + 0.5 * Math.cos(h * 2.1))));
      ctx.fillStyle = cg; ctx.fill('evenodd');
    });
    ctx.restore();
    ctx.save(); ctx.lineWidth = Math.max(1, S0 * 0.004);
    var eg = ctx.createLinearGradient(cx - Ro, cy - Ro, cx + Ro, cy + Ro); eg.addColorStop(0, '#d4d7e2'); eg.addColorStop(0.5, '#4a4f5c'); eg.addColorStop(1, '#8a8f9c');
    ctx.strokeStyle = eg; ctx.beginPath(); ctx.arc(cx, cy, Ro, 0, TAU); ctx.stroke(); ctx.beginPath(); ctx.arc(cx, cy, Ri, 0, TAU); ctx.stroke();
    ctx.restore();
    ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.lineCap = 'round'; ctx.lineJoin = 'round';
    var rot = t * 0.12, cell = (Ro - Ri) * 0.26;
    for (var i = 0; i < RUNES.length; i++) {
      var a = rot + i / RUNES.length * TAU, pulse = Math.pow(0.5 + 0.5 * Math.cos(a - t * 1.1), 8);
      var col = rgba(tint.map(function (v) { return Math.min(255, v * 1.1 + 80); }), 0.4 + 0.6 * pulse);
      ctx.save(); ctx.translate(cx + Math.cos(a) * Rm, cy + Math.sin(a) * Rm); ctx.rotate(a + Math.PI / 2);
      ctx.shadowColor = rgba(tint, 0.9); ctx.shadowBlur = S0 * (0.01 + 0.02 * pulse);
      ctx.globalAlpha = 0.45 + 0.55 * pulse; ctx.strokeStyle = col; ctx.lineWidth = Math.max(1, S0 * 0.0055);
      ctx.beginPath();
      RUNES[i].forEach(function (st) { ctx.moveTo(st[0][0] * cell * 0.6, st[0][1] * cell); ctx.lineTo(st[1][0] * cell * 0.6, st[1][1] * cell); });
      ctx.stroke(); ctx.restore();
    }
    ctx.globalAlpha = 1;
    // Ticks counter-turning outside the band, and a fine sigil circle turning inside it, over the picture's edge.
    ctx.strokeStyle = rgba(tint.map(function (v) { return Math.min(255, v + 110); }), 0.45); ctx.lineWidth = Math.max(1, S0 * 0.003); ctx.beginPath();
    for (var k = 0; k < 72; k++) { var b = -t * 0.06 + k / 72 * TAU, l = k % 6 ? 0.012 : 0.022; ctx.moveTo(cx + Math.cos(b) * (Ro + S0 * 0.006), cy + Math.sin(b) * (Ro + S0 * 0.006)); ctx.lineTo(cx + Math.cos(b) * (Ro + S0 * (0.006 + l)), cy + Math.sin(b) * (Ro + S0 * (0.006 + l))); }
    ctx.stroke();
    var Rs = Ri - S0 * 0.016;
    ctx.strokeStyle = rgba(tint.map(function (v) { return Math.min(255, v + 100); }), 0.35); ctx.lineWidth = Math.max(0.6, S0 * 0.0018);
    ctx.beginPath(); ctx.arc(cx, cy, Rs, 0, TAU);
    for (var g2 = 0; g2 < 12; g2++) { var gb = t * 0.09 + g2 / 12 * TAU, gx = cx + Math.cos(gb) * Rs, gy = cy + Math.sin(gb) * Rs; ctx.moveTo(gx + S0 * 0.008, gy); ctx.arc(gx, gy, S0 * 0.008, 0, TAU); }
    ctx.stroke();
    // The three crystals ride the outer edge; now and then lightning leaps from one to the next.
    var cr = Ro + S0 * 0.012, cps = [];
    for (var c2 = 0; c2 < 3; c2++) { var ca2 = t * 0.4 + c2 * TAU / 3; cps.push([cx + Math.cos(ca2) * cr, cy + Math.sin(ca2) * cr, ca2]); }
    var slot = Math.floor(t * 1.6), tf = fract(t * 1.6), flick = Math.floor(t * 30);
    [0, 1, 2].forEach(function (bi) {
      if (hash(slot, bi + 3) > Math.min(0.9, [0.45, 0.22, 0.32][bi] * pal.bolts) || tf > 0.26) return;
      var alpha = (1 - tf / 0.26) * (0.6 + 0.4 * hash(flick, 6 + bi));
      if (bi === 2) { // a crystal-to-crystal arc, bowing outward
        var j = Math.floor(hash(slot, 9) * 3), A1 = cps[j], A2 = cps[(j + 1) % 3], am = (A1[2] + A2[2] + (A2[2] < A1[2] ? TAU : 0)) / 2;
        arcBolt(ctx, A1, A2, [cx + Math.cos(am) * (Ro + S0 * 0.02), cy + Math.sin(am) * (Ro + S0 * 0.02)], flick + 5, S0, tint, alpha);
        return;
      }
      var a1 = hash(slot + bi * 1000, 1) * TAU, a2 = a1 + (0.5 + 0.7 * hash(slot + bi * 1000, 2)) * (hash(slot, 5 + bi) > 0.5 ? 1 : -1), am2 = (a1 + a2) / 2;
      arcBolt(ctx, [cx + Math.cos(a1) * Rm, cy + Math.sin(a1) * Rm], [cx + Math.cos(a2) * Rm, cy + Math.sin(a2) * Rm],
        [cx + Math.cos(am2) * (Ro + S0 * 0.03), cy + Math.sin(am2) * (Ro + S0 * 0.03)], flick + bi * 13, S0, tint, alpha);
    });
    // Motes of light drifting round, twinkling.
    for (var m = 0; m < 34; m++) {
      var ma = hash(m, 1) * TAU + t * (0.05 + 0.1 * hash(m, 2)) * (m % 2 ? 1 : -1), mr = S0 * (0.32 + 0.2 * hash(m, 3));
      var tw = Math.pow(0.5 + 0.5 * Math.sin(t * (1.5 + hash(m, 4) * 3) + m * 2), 3), mx = cx + Math.cos(ma) * mr, my = cy + Math.sin(ma) * mr;
      var ms = S0 * (0.004 + 0.004 * hash(m, 5)) * (0.4 + tw);
      ctx.fillStyle = rgba(tint.map(function (v) { return Math.min(255, v + 140); }), tw);
      ctx.beginPath(); ctx.arc(mx, my, ms, 0, TAU); ctx.fill();
      if (tw > 0.6) { ctx.strokeStyle = 'rgba(255,255,255,' + (tw - 0.6) * 1.5 + ')'; ctx.lineWidth = Math.max(0.5, S0 * 0.0012); ctx.beginPath(); ctx.moveTo(mx - ms * 3, my); ctx.lineTo(mx + ms * 3, my); ctx.moveTo(mx, my - ms * 3); ctx.lineTo(mx, my + ms * 3); ctx.stroke(); }
    }
    // Each crystal trails a fading arc of light.
    cps.forEach(function (p, ci) {
      for (var q = 1; q <= 8; q++) {
        var qa = p[2] - q * 0.035;
        ctx.fillStyle = rgba(tint.map(function (v) { return Math.min(255, v + 120); }), 0.3 * (1 - q / 9));
        ctx.beginPath(); ctx.arc(cx + Math.cos(qa) * cr, cy + Math.sin(qa) * cr, S0 * 0.008 * (1 - q / 10), 0, TAU); ctx.fill();
      }
    });
    ctx.restore();
    cps.forEach(function (p, ci) { crystal(ctx, p[0], p[1], S0 * 0.044, t * 1.7 + ci * 2, hueOf(tint), 0.8 + 0.2 * Math.sin(t * 3 + ci), satOf(tint)); });
    // The picture's edge picks up the glow.
    ctx.save(); ctx.beginPath(); ctx.arc(cx, cy, Ri, 0, TAU); ctx.clip();
    var gc = tint;
    var ig = ctx.createRadialGradient(cx, cy, Ri * 0.7, cx, cy, Ri); ig.addColorStop(0, rgba(gc, 0)); ig.addColorStop(1, rgba(gc, 0.2 + 0.06 * Math.sin(t * 1.3)));
    ctx.fillStyle = ig; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
  }
  /* ---------- Paper: a cut-out storybook token, drawn by hand, its lines boiling like hand-drawn animation ---------- */
  function wob(seed, a, amt) { return amt * (qn(Math.cos(a) * 3 + seed * 13.7, Math.sin(a) * 3 + seed * 7.1) - 0.5); }
  function ring(ctx, cx, cy, r, seed, amt, n) {
    ctx.beginPath();
    for (var k = 0; k <= n; k++) { var a = k / n * TAU, rr = r + wob(seed, a, amt); if (k) ctx.lineTo(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr); else ctx.moveTo(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr); }
    ctx.closePath();
  }
  // pal: the paper, the ink, the banner and the lettering for one kind of storybook.
  function paperFront(ctx, G, t, playing, pal) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, boil = playing ? Math.floor(t * 8) % 3 : 0, Ro = S0 * 0.47, Ri = S0 * 0.39;
    var ink = pal.ink;
    // The paper ring, deckle-edged, inked on both edges.
    ctx.save();
    ctx.beginPath();
    for (var k = 0; k <= 200; k++) { var a = k / 200 * TAU, rr = Ro + wob(9, a * 4, S0 * 0.014) + S0 * 0.004 * hash(k, 3); if (k) ctx.lineTo(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr); else ctx.moveTo(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr); }
    ctx.closePath(); ctx.arc(cx, cy, Ri, 0, TAU, true);
    ctx.fillStyle = ctx.createPattern(paperTexture(pal.paper), 'repeat'); ctx.fill('evenodd');
    // Cross-hatching in the shaded lower right of the ring.
    ctx.clip('evenodd');
    ctx.strokeStyle = pal.hatch; ctx.lineWidth = Math.max(0.7, S0 * 0.0028); ctx.beginPath();
    for (var h = -0.12; h <= 0.5; h += 0.03) {
      var jx = (hash(h * 100, boil) - 0.5) * S0 * 0.01;
      ctx.moveTo(cx + h * S0 + jx, cy + S0 * 0.05); ctx.lineTo(cx + h * S0 + S0 * 0.5 + jx, cy + S0 * 0.55);
    }
    ctx.stroke();
    ctx.restore();
    ctx.save(); ctx.strokeStyle = ink; ctx.lineJoin = 'round';
    ctx.lineWidth = Math.max(1.2, S0 * 0.008); ring(ctx, cx, cy, Ro - S0 * 0.006, boil * 3 + 1, S0 * 0.008, 140); ctx.stroke();
    ctx.lineWidth = Math.max(1.4, S0 * 0.011); ring(ctx, cx, cy, Ri + S0 * 0.002, boil * 3 + 2, S0 * 0.006, 140); ctx.stroke();
    ctx.restore();
    // A ribbon banner across the bottom with the creature's name, waving.
    var bw = S0 * 0.74, bh = S0 * 0.11, by = cy + S0 * 0.34, bx = cx - bw / 2, n = 24, amp = S0 * (playing ? 0.008 : 0.005);
    var wave = function (x) { return Math.sin(x / bw * 6 + t * 2.2) * amp; };
    var col = tc(rgb(pal.banner));
    ctx.save(); ctx.lineJoin = 'round';
    [-1, 1].forEach(function (side) { // folded tails behind the banner
      var ex = side < 0 ? bx : bx + bw, tx = ex + side * S0 * 0.09;
      ctx.beginPath(); ctx.moveTo(ex - side * S0 * 0.04, by + wave(ex - bx) + bh * 0.25); ctx.lineTo(tx, by + bh * 0.2 + wave(ex - bx));
      ctx.lineTo(tx - side * S0 * 0.03, by + bh * 0.72 + wave(ex - bx)); ctx.lineTo(tx, by + bh * 1.2 + wave(ex - bx)); ctx.lineTo(ex - side * S0 * 0.04, by + bh * 1.2 + wave(ex - bx)); ctx.closePath();
      ctx.fillStyle = rgba(col.map(function (v) { return v * 0.62; }), 1); ctx.fill(); ctx.lineWidth = Math.max(1, S0 * 0.006); ctx.strokeStyle = ink; ctx.stroke();
    });
    ctx.beginPath();
    for (var i = 0; i <= n; i++) { var x = bx + bw * i / n; ctx[i ? 'lineTo' : 'moveTo'](x, by + wave(x - bx) + wob(boil, i, S0 * 0.004)); }
    for (i = n; i >= 0; i--) { x = bx + bw * i / n; ctx.lineTo(x, by + bh + wave(x - bx) + wob(boil + 5, i, S0 * 0.004)); }
    ctx.closePath(); ctx.fillStyle = rgba(col, 1); ctx.fill(); ctx.lineWidth = Math.max(1.2, S0 * 0.008); ctx.strokeStyle = ink; ctx.stroke();
    // The creature's name, made smaller to fit a long one, and cut short only past that.
    var name = String(S.name || ''), fs = bh * 0.62, font = function (px) { ctx.font = '400 ' + Math.round(px) + 'px "IM Fell English SC", Georgia, serif'; };
    ctx.fillStyle = pal.letters; ctx.textAlign = 'center'; ctx.textBaseline = 'middle'; font(fs);
    var fit = bw * 0.84, wide = ctx.measureText(name).width;
    if (wide > fit) { fs = Math.max(bh * 0.36, fs * fit / wide); font(fs); }
    while (name.length > 1 && ctx.measureText(name).width > fit) name = name.slice(0, -2) + '\u2026';
    var total = ctx.measureText(name).width, xx = cx - total / 2;
    for (var c = 0; c < name.length; c++) {
      var ch = name[c], wch = ctx.measureText(ch).width;
      ctx.fillText(ch, xx + wch / 2, by + bh * 0.54 + wave(xx + wch / 2 - bx)); xx += wch;
    }
    ctx.restore();
  }
  function paperBack(ctx, G) { // the cut-out's shadow on the table, offset like paper lifted off a page
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2;
    ctx.save(); ctx.filter = 'blur(' + Math.max(1, S0 * 0.012) + 'px)'; ctx.fillStyle = 'rgba(0,0,0,.4)';
    ctx.beginPath(); ctx.arc(cx + S0 * 0.012, cy + S0 * 0.02, S0 * 0.47, 0, TAU); ctx.fill(); ctx.restore();
  }
  var STORYBOOKS = {
    cream: { paper: 'washi', ink: '#2a221c', hatch: 'rgba(42,34,28,.55)', banner: '#8f2a2a', letters: '#f4ead2' },
    forest: { paper: 'kraft', ink: '#26301e', hatch: 'rgba(38,48,30,.55)', banner: '#2f6a4a', letters: '#f1ead6' },
    night: { paper: 'night', ink: '#e8e2d0', hatch: 'rgba(232,226,208,.4)', banner: '#2c4a8c', letters: '#f3e3b0' }
  };
  /* ---------- Acid ----------
     A ring of caustic acid pooled on the picture's edge and eating into it. The liquid is a thin, luminous
     yellow-lime film: deepest green where it lies thick, pale and see-through where it thins, with slow currents
     drawn out along the flow and fine ripples that throw crisp glints. It is shaded as a surface (small height
     field per frame, normals, a mirrored window), but its colour comes from how much liquid the light passes
     through, not from diffuse shading, so it reads as wet and clear rather than as rubber. Along its contact
     lines it froths: clusters of bubbles form, rise, swell and pop, and now and then a large one swells and
     bursts. Where it touches the picture and the map the surface is etched: a ragged black char, a stain in the
     material, pits, and runs of stain trailing outward; fingers of liquid creep out over it and drips run on.
     Pale fumes lift off. Everything is in token units; the frame is drawn at AC_SC of the canvas so the fumes,
     fingers and char have room. */
  var AC_SC = 0.88, AC_EXT = 1.14, AC_N = 336, AC_E = 1024, AC_NT = 13, AC_NB0 = 8, AC_FLOW = 0.03;
  var AC_BODY = [[0, [218, 252, 104]], [0.12, [192, 248, 62]], [0.34, [152, 236, 36]], [0.6, [100, 208, 30]], [0.9, [46, 162, 24]], [1.2, [20, 114, 18]], [1.5, [8, 72, 12]]];
  // The colours of the effects drawn over the liquid, in the design's own colours: the sprites are made once from these and graded as layers.
  var AC_RAW = { spec: [252, 255, 226], glow: [176, 246, 62], deep: [8, 52, 8], foam: [226, 250, 150], vap: [206, 240, 140], drop: [170, 240, 60] };
  var acPals = {}, acSpr = {};
  function acSprite(name, make) { if (!acSpr[name]) acSpr[name] = make(); return graded(name, acSpr[name]); }
  function acPal() {
    var id = gradeId();
    if (acPals[id]) return acPals[id];
    var lut = new Float32Array(97 * 3);
    for (var i = 0; i <= 96; i++) { var c = tc(rampAt(AC_BODY, i / 96 * 1.5)); lut[i * 3] = c[0]; lut[i * 3 + 1] = c[1]; lut[i * 3 + 2] = c[2]; }
    return (acPals[id] = { lut: lut, spec: tc([252, 255, 226]), glow: tc([176, 246, 62]), deep: tc([8, 52, 8]), foam: tc([226, 250, 150]), vap: tc([206, 240, 140]), drop: tc([170, 240, 60]) });
  }
  // The contact lines. The inner one is where the picture's edge has been eaten and never moves; the outer one
  // wavers, and slow fingers of liquid creep out from fixed places along it (each cycles out, holds and draws back).
  var acEd = null;
  function acEdgeStatic() {
    if (acEd) return acEd;
    var E = AC_E, e = acEd = { ri: new Float32Array(E), ro0: new Float32Array(E), ro: new Float32Array(E), ki: new Float32Array(E), ko: new Float32Array(E), env: new Float32Array(E),
      ta: [], tw: [], tl: [], tp: [], tf: [] }, i, k;
    e.tr = []; e.tg = [];
    for (k = 0; k < AC_NT; k++) {
      var run = k >= AC_NB0;           // the first few are broad lobes, the rest narrow runs that end in a bead
      e.tr[k] = run; e.tg[k] = 0; e.ta[k] = (k + 0.15 + 0.7 * hash(k, 31)) / AC_NT;
      e.tw[k] = run ? 0.011 + 0.005 * hash(k, 32) : 0.024 + 0.016 * hash(k, 32); e.tl[k] = run ? 0.022 + 0.02 * hash(k, 33) : 0.01 + 0.018 * hash(k, 33);
      e.tp[k] = (run ? 8 : 11) + 8 * hash(k, 34); e.tf[k] = hash(k, 35);
    }
    for (i = 0; i < E; i++) {
      var a = i / E;
      e.ri[i] = 0.386 + 0.01 * (pvn(a * 4 + 2, 0.5, 4) - 0.5) * 2 + 0.006 * (pvn(a * 16 + 5, 0.5, 16) - 0.5) * 2 + 0.003 * (pvn(a * 64 + 1, 0.5, 64) - 0.5) * 2 + 0.0012 * (pvn(a * 128, 0.5, 128) - 0.5) * 2;
      e.ro0[i] = 0.468 + 0.032 * (pvn(a * 4, 1.5, 4) - 0.5) * 2 + 0.02 * (pvn(a * 8 + 3, 2.5, 8) - 0.5) * 2 + 0.006 * (pvn(a * 32, 3.5, 32) - 0.5) * 2 + 0.0022 * (pvn(a * 128, 3.5, 128) - 0.5) * 2;
      e.env[i] = e.ro0[i];
    }
    for (k = 0; k < AC_NT; k++) { e.tl[k] = Math.min(e.tl[k], Math.max(0.008, 0.534 - e.ro0[Math.round(e.ta[k] * E) & (E - 1)])); acTongue(e.env, e.ta[k], e.tw[k], e.tl[k] * 0.8); }
    var eb = new Float32Array(E);
    for (i = 0; i < E; i++) { var acc = 0; for (k = -8; k <= 8; k++) acc += e.env[(i + k + E) & (E - 1)] * (9 - Math.abs(k)); eb[i] = acc / 81; }
    for (i = 0; i < E; i++) e.env[i] = Math.max(eb[i], e.ro0[i]);
    var sl = AC_E / (TAU * 0.47);
    for (i = 0; i < E; i++) { var s = (e.ri[(i + 1) & (E - 1)] - e.ri[(i + E - 1) & (E - 1)]) * 0.5 * sl; e.ki[i] = 1 / Math.sqrt(1 + s * s); }
    return e;
  }
  // One finger: a flat-sided tongue with a rounded tip, added to an outer-edge table.
  function acTongue(tab, a0, w, L) {
    var E = AC_E, span = Math.ceil(w / (TAU * 0.47) * E) + 1;
    for (var j = -span; j <= span; j++) {
      var ii = (Math.round(a0 * E) + j + E) & (E - 1), s = (j / E + (Math.round(a0 * E) / E - a0)) * TAU * 0.47, u = Math.abs(s) / w;
      if (u < 1) tab[ii] += L * (1 - u * u);
    }
  }
  function acEdgeFrame(t) {
    var e = acEdgeStatic(), E = AC_E, ro = e.ro, ko = e.ko, i, k;
    for (i = 0; i < E; i++) { var a = i / E; ro[i] = e.ro0[i] + 0.0045 * (pvn(a * 16 - t * AC_FLOW * 16, t * 0.12, 16) - 0.5) * 2; }
    for (k = 0; k < AC_NT; k++) {
      var cyc = t / e.tp[k] + e.tf[k], f = cyc - Math.floor(cyc), g = smooth(0, 0.42, f) * (1 - smooth(0.62, 1, f));
      e.tg[k] = g;
      if (g > 0.01) acTongue(ro, e.ta[k], e.tw[k] * (0.7 + 0.3 * g), e.tl[k] * g);
    }
    var sl = E / (TAU * 0.47);
    for (i = 0; i < E; i++) { var s = (ro[(i + 1) & (E - 1)] - ro[(i + E - 1) & (E - 1)]) * 0.5 * sl; ko[i] = 1 / Math.sqrt(1 + s * s); }
    return e;
  }
  // The softbox the liquid mirrors: a bar of light at the top left. Where the surface tilts to show it, it is a
  // sharp-edged bright patch that bends with the surface; a dimmer one low on the right gives the far side its sheen.
  function acWindow(Rx, Ry) {
    var kx = Rx + 0.56, ky = Ry + 0.69, wx = Math.abs(kx * 0.78 - ky * 0.62), wy = Math.abs(kx * 0.62 + ky * 0.78);
    if (wx >= 0.2 || wy >= 0.085) return 0;
    var ax = wx < 0.16 ? 1 : (0.2 - wx) / 0.04, ay = wy < 0.06 ? 1 : (0.085 - wy) / 0.025;
    return ax * ax * (3 - 2 * ax) * ay * ay * (3 - 2 * ay);
  }
  function acWindow2(Rx, Ry) {
    var kx = Rx - 0.5, ky = Ry - 0.55, d = (kx * kx + ky * ky) / 0.045;
    return d < 1 ? (1 - d) * (1 - d) : 0;
  }
  // Worked out once: every texel the liquid can reach, with its angle round the ring and its radius.
  var acF = { q: -1, pk: null, PX: null, cv: null, img: null, H: null, TH: null, D: null, AL: null };
  function acSetup(F) {
    var N = AC_N, list = [], x, y;
    for (y = 2; y < N - 2; y++) for (x = 2; x < N - 2; x++) {
      var X = ((x + 0.5) / N - 0.5) * AC_EXT, Y = ((y + 0.5) / N - 0.5) * AC_EXT, r = Math.hypot(X, Y);
      if (r < 0.355 || r > 0.555) continue;
      var a = Math.atan2(Y, X) / TAU + 0.5; if (a >= 1) a = 0.99999;
      list.push(y * N + x, a, r);
    }
    F.PX = new Float32Array(list); F.cv = document.createElement('canvas'); F.cv.width = F.cv.height = N;
    F.img = F.cv.getContext('2d').createImageData(N, N); F.H = new Float32Array(N * N); F.TH = new Float32Array(N * N); F.D = new Float32Array(N * N); F.AL = new Int32Array(list.length / 3);
  }
  // Froth: the liquid's surface is dotted with tiny bubbles. They sit in a grid of cells that turns with the current,
  // each cell holding at most one bubble that grows, hangs, bursts in a flash and leaves the cell empty for a while,
  // so thousands of them cost one lookup per texel. Light is the same as for the big ones: a dark rim away from
  // the light, a lime crescent focused onto the far side, and a pin of the key light.
  var acFo = { rim: 0, cau: 0, hl: 0 };
  function acFoam(u, v, ex, t, fm, seed) {
    var iu = Math.floor(u), iv = Math.floor(v), fu = u - iu, fv = v - iv, life = 1.5 + 2.4 * lat(iu + seed * 3, iv + 5 + seed);
    var cyc = t / life + lat(iu * 3 + seed, iv + 11 + seed), cn = Math.floor(cyc), ph = cyc - cn;
    acFo.rim = acFo.cau = acFo.hl = 0;
    if (ph > 0.82 || lat(iu + cn * 5 + seed * 17, iv + cn * 9 + 3) > fm) return false;
    var rf = 0.17 + 0.17 * lat(iu + cn * 7 + 31, iv + cn * 3 + seed * 5), cu = rf + (1 - 2 * rf) * lat(iu + cn * 2 + 7, iv + seed), cv = rf + (1 - 2 * rf) * lat(iu + 13 + seed, iv + cn * 4 + 1);
    var dx = fu - cu, dy = fv - cv, dd = Math.sqrt(dx * dx + dy * dy), L = LIGHT, lxy = Math.hypot(L[0], L[1]);
    if (ph < 0.72) {
      var gr = ph < 0.3 ? 0.2 + 0.8 * smooth(0, 0.3, ph) : 1, rad = rf * gr, rho = dd / rad;
      if (rho >= 1) return false;
      var sx = (dx * ex[0] + dy * ex[2]) / (dd || 1), sy = (dx * ex[1] + dy * ex[3]) / (dd || 1), face = (sx * L[0] + sy * L[1]) / lxy;   // which way the dome tilts, in screen terms
      var rm = smooth(0.5, 0.97, rho), lf = face > 0 ? 0 : -face, cw = 1 - (rho - 0.6) * (rho - 0.6) * 11;
      acFo.rim = rm * rm * (0.3 + 0.6 * smooth(-0.2, 0.9, -face)) * (0.4 + 0.6 * gr);
      acFo.cau = cw > 0 ? lf * Math.sqrt(lf) * cw * cw * 0.75 * gr : 0;
      var hx = sx * rho + 0.42 * L[0] / lxy, hy = sy * rho + 0.42 * L[1] / lxy, h2 = 1 - (hx * hx + hy * hy) / 0.14, e2 = (rho - 0.95) / 0.07;
      acFo.hl = (h2 > 0 ? h2 * h2 * 1.1 * gr : 0) + (e2 > -1 && e2 < 1 ? (1 - e2 * e2) * smooth(0.3, 0.9, face) * 0.3 : 0);
    } else {
      var q = (ph - 0.72) / 0.1, rr = rf * (1 + 0.35 * q), e3 = (dd - rr * 0.95) / (rf * 0.18);
      acFo.hl = e3 > -1 && e3 < 1 ? (1 - e3 * e3) * (1 - q) * 0.8 : 0;
    }
    return true;
  }
  // The liquid for one frame, into a texture of the token's surroundings. Height gives the surface (its edge is
  // a rounded bead with a raised lip), a flow tone gives how deep the liquid lies, and both move with the current.
  function acField(t) {
    var F = acF, N = AC_N, q = Math.round(t * 30), gid = gradeId();
    if (F.q === q && F.pk === gid) return F.cv;
    if (!F.PX) acSetup(F);
    var e = acEdgeFrame(t), RI = e.ri, RO = e.ro, KI = e.ki, KO = e.ko, E = AC_E, pal = acPal(), lut = pal.lut;
    var PX = F.PX, H = F.H, TH = F.TH, D = F.D, AL = F.AL, nal = 0, om = t * AC_FLOW, t1 = t * 0.05, k, j, i, d = F.img.data;
    H.fill(0); d.fill(0);
    for (k = 0; k < PX.length; k += 3) {
      i = PX[k]; var a = PX[k + 1], r = PX[k + 2], ai = (a * E) | 0, dO = (RO[ai] - r) * KO[ai], dI = (r - RI[ai]) * KI[ai], dd = dO < dI ? dO : dI;
      if (dd < -0.004) continue;
      var wp = pvn(a * 8 - om * 8 + 3.1, r * 9 + t1, 8) - 0.5, wq = pvn(a * 16 - om * 15 + 1.7, r * 22 - t1, 16) - 0.5;
      var broad = pfbm(a * 16 - om * 16 + wp * 1.8, r * 14 + t1 * 0.7 + 4, 16, 2);
      var vein = pvn(a * 32 - om * 35 + wp * 3 + wq * 1.5, r * 48 + 7 + t1 * 0.4 + wq * 2, 32), vn = 1 - Math.abs(2 * vein - 1), vn2 = vn * vn * vn;
      var rip = pvn(a * 128 - om * 140 + wp * 5 + wq * 3 + r * 53, r * 47 + t * 0.35 + wq * 4, 128);
      var x = dd * 71, Pe = x >= 1 ? 1 : x <= 0 ? 0 : Math.sqrt(1 - (1 - x) * (1 - x)), xc = dd * 28, Pc = xc >= 1 ? 1 : xc <= 0 ? 0 : xc * (2 - xc);
      var lx = (dd - 0.0045) * 300, lip = lx > -1 && lx < 1 ? (1 - lx * lx) * (1 - lx * lx) : 0;
      H[i] = Pe * (0.8 + 0.42 * (broad - 0.5) + 0.07 * (rip - 0.5) + 0.05 * (vein - 0.5)) + 0.2 * lip * (0.3 + 1.1 * pvn(a * 64 - om * 64, r * 5 + 3.3, 64));
      var tone = clamp((broad - 0.5) * 2.1 + 0.5, 0, 1); TH[i] = Pc * (0.14 + 0.95 * tone) * (1 - 0.6 * vn2 * Pc); D[i] = dd; AL[nal++] = k;
    }
    var KS = 14, L = LIGHT, Hf = HALF, lxy = Math.hypot(L[0], L[1]), sp = pal.spec, gl = pal.glow, dp = pal.deep, ex = [0, 0, 0, 0];
    for (j = 0; j < nal; j++) {
      k = AL[j]; i = PX[k]; var o = i * 4, th = TH[i], dv = D[i], a2 = PX[k + 1], r2 = PX[k + 2];
      var gx = (H[i + 1] - H[i - 1]) * KS, gy = (H[i + N] - H[i - N]) * KS, inv = 1 / Math.sqrt(1 + gx * gx + gy * gy), Nx = -gx * inv, Ny = -gy * inv, Nz = inv;
      var diff = Math.max(0, Nx * L[0] + Ny * L[1] + Nz * L[2]), hh = Nx * Hf[0] + Ny * Hf[1] + Nz * Hf[2];
      var spec = hh > 0.93 ? Math.pow(hh, 150) * 1.3 : 0, tilt = 1 - Nz, fre = 0.1 + 0.9 * tilt * tilt;
      var win = acWindow(2 * Nz * Nx, 2 * Nz * Ny) * fre * 1.5 + acWindow2(2 * Nz * Nx, 2 * Nz * Ny) * fre * 0.5;
      var lee = Math.max(0, -(Nx * L[0] + Ny * L[1]) / lxy) * tilt * 2.2;
      var ti = Math.min(96, Math.round(Math.min(1.5, th * 1.15) / 1.5 * 96)) * 3, lit = 0.8 + 0.3 * diff, hl = spec + win;
      var cr = lut[ti] * lit + sp[0] * hl + gl[0] * lee * 0.4, cg = lut[ti + 1] * lit + sp[1] * hl + gl[1] * lee * 0.4, cb = lut[ti + 2] * lit + sp[2] * hl + gl[2] * lee * 0.4;
      var cover = smooth(-0.0025, 0.0025, dv), al = (0.72 + 0.26 * Math.min(1, th * 2.4)) * cover, hlc = (hl > 1 ? 1 : hl) * 0.95 * cover;
      if (hlc > al) al = hlc;
      // Froth, thickest in patches and toward the contact lines.
      var fm = smooth(0.45, 0.8, 0.72 * pvn(a2 * 16 - om * 16 + 5, r2 * 24 + t1, 16) + 0.4 * (1 - smooth(0, 0.045, dv))) * 0.8 * smooth(0.002, 0.012, dv);
      if (fm > 0.03) {
        var X = ((i % N + 0.5) / N - 0.5) * AC_EXT, Y = (((i / N) | 0) + 0.5) / N - 0.5, ct = X / r2, st = (Y * AC_EXT) / r2;
        ex[0] = -st; ex[1] = ct; ex[2] = ct; ex[3] = st;                // tangent and radial directions on screen
        if (acFoam(a2 * 192 - om * 192, r2 / 0.0141, ex, t, fm, 1)) {
          var rimA = acFo.rim * 0.62, ca = acFo.cau * 0.7, hb = acFo.hl > 1 ? 1 : acFo.hl;
          cr = (cr * (1 - rimA) + dp[0] * rimA + gl[0] * ca) * (1 - hb) + sp[0] * hb; cg = (cg * (1 - rimA) + dp[1] * rimA + gl[1] * ca) * (1 - hb) + sp[1] * hb; cb = (cb * (1 - rimA) + dp[2] * rimA + gl[2] * ca) * (1 - hb) + sp[2] * hb;
          al += (cover - al) * 0.6 * Math.min(1, rimA + ca + hb);
        }
      }
      d[o] = cr; d[o + 1] = cg; d[o + 2] = cb; d[o + 3] = 255 * al;
    }
    F.cv.getContext('2d').putImageData(F.img, 0, 0); F.q = q; F.pk = gid;
    return F.cv;
  }
  // The etched ground, drawn once in the design's own colours and then graded. Over the picture: a ragged black
  // char bitten into its edge, with fingers where the acid ran in, a burnt lip, a stain yellowed into the picture
  // beyond it and craters that thin out inward. Under the liquid it is all char. Over the map: a mottled scorch
  // whose edge is ragged and pitted, with stain trailing outward in streaks where the acid has crept.
  function acCrater(X, Y, K, pg, rmin, rmax, c) { // a pit in the cell grid: dark floor, lit wall on the side facing the light, shadowed wall opposite
    var f = voronoi(X * K, Y * K);
    if (VOR.id >= pg) return false;
    var pr = rmin + (rmax - rmin) * VOR.id / pg, rho = f.f1 / pr;
    if (rho >= 1.25) return false;
    var dx = X * K - f.cx, dy = Y * K - f.cy, il = 1 / (f.f1 || 1), face = (dx * il * LIGHT[0] + dy * il * LIGHT[1]) / 0.8;
    if (rho < 1) {
      var wall = smooth(0.5, 1, rho), lit = wall * smooth(-0.1, 0.8, -face), dk = wall * smooth(-0.1, 0.8, face);
      c[0] = 7 + 100 * lit * 0.85; c[1] = 6 + 84 * lit * 0.85; c[2] = 4 + 30 * lit * 0.85; c[3] = 0.95 * (1 - 0.3 * lit) - 0.0 * dk;
    } else { var lp = smooth(1.25, 1.0, rho) * smooth(0.1, 0.8, -face); c[0] = 92; c[1] = 78; c[2] = 34; c[3] = 0.45 * lp; }
    return c[3] > 0.01;
  }
  var acC = [0, 0, 0, 0];
  function acScorchDraw(w, h) {
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), im = g.createImageData(w, h), d = im.data, bs = Math.round(w * AC_SC), E = acEdgeStatic(), cx = w / 2, cy = h / 2, x, y;
    for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
      var X = (x + 0.5 - cx) / bs, Y = (y + 0.5 - cy) / bs, r = Math.hypot(X, Y);
      if (r < 0.3 || r > 0.58) continue;
      var a = Math.atan2(Y, X) / TAU + 0.5; if (a >= 1) a = 0.99999;
      var ei = (a * AC_E) | 0, ri = E.ri[ei], env = E.env[ei], o = (y * w + x) * 4, cr = 0, cg = 0, cb = 0, al = 0, n1, n2, n3, v, m, ch;
      if (r < ri) {
        var di = ri - r;
        n1 = fbm(X * 10 + 3.7, Y * 10 + 1.2, 4); n2 = vnoise(X * 80, Y * 80); n3 = fbm(X * 36 + 5, Y * 36, 3);
        var fing = pvn(a * 128 + 0.3, r * 22, 128), lim = 0.0015 + 0.028 * smooth(0.36, 0.78, n1) + 0.014 * smooth(0.5, 0.95, fing) * smooth(0.3, 0.7, n1) + 0.01 * (n3 - 0.5);
        m = lim - di + 0.0045 * (n2 - 0.5);
        var stn = 0.36 * (1 - smooth(0.008, 0.075, di + 0.03 * (n1 - 0.5))) * (0.45 + 0.9 * n3);
        if (stn > 0.004) { cr = 112; cg = 118; cb = 26; al = stn; }
        if (di < 0.1) {
          var pg = 0.8 * (1 - smooth(0.004, 0.09, di)) * (0.4 + 0.9 * n1);
          if (acCrater(X, Y, 56, pg, 0.16, 0.42, acC) || acCrater(X, Y, 120, pg * 0.8, 0.14, 0.34, acC)) { var ca = acC[3]; cr = cr * (1 - ca) + acC[0] * ca; cg = cg * (1 - ca) + acC[1] * ca; cb = cb * (1 - ca) + acC[2] * ca; al = al + (1 - al) * ca; }
        }
        if (m > -0.007) { var lipa = smooth(-0.007, 0, m) * 0.75; if (lipa > al * 0.6) { cr = 104; cg = 78; cb = 30; al = Math.max(al, lipa); } }
        if (m > 0) {
          ch = smooth(0, 0.0014, m); v = 6 + 20 * vnoise(X * 50, Y * 50) * (0.5 + 0.8 * n3);
          cr = cr * (1 - ch) + v * 1.05 * ch; cg = cg * (1 - ch) + v * 0.9 * ch; cb = cb * (1 - ch) + v * 0.66 * ch; al = al + (0.97 - al) * ch;
        }
      } else {
        var dOut = r - env;
        n1 = fbm(X * 9 + 1.3, Y * 9 + 5.1, 4); n2 = vnoise(X * 70 + 3, Y * 70); n3 = fbm(X * 28 + 2, Y * 28 + 8, 3);
        var streak = pvn(a * 128 + 0.5, r * 14, 128), lo = 0.0015 + 0.024 * smooth(0.36, 0.78, n1) + 0.013 * smooth(0.4, 0.95, streak) * smooth(0.3, 0.7, n1) + 0.008 * (n3 - 0.5);
        lo = Math.min(lo, Math.max(0.004, 0.542 - env));
        m = lo - dOut + 0.004 * (n2 - 0.5);
        var halo = 0.3 * (1 - smooth(0.0, 0.034, -m)) * (0.4 + 0.8 * n3) * (1 - smooth(0.515, 0.55, r));
        if (halo > 0.004 && m < 0) { cr = 70; cg = 62; cb = 22; al = halo; }
        if (m > -0.009) { var lp2 = smooth(-0.009, 0, m) * 0.65; if (lp2 > al) { cr = 92; cg = 70; cb = 26; al = lp2; } }
        if (m > 0) {
          ch = smooth(0, 0.0014, m); v = 8 + 24 * n3 * (0.5 + 0.7 * n2);
          var hole = 0;
          if (m < 0.014 + 0.012 * n1) {
            var pg2 = 0.85 * (1 - smooth(0.0, 0.02, m));
            if (acCrater(X, Y, 70, pg2, 0.14, 0.4, acC)) hole = 0.85;
          }
          cr = cr * (1 - ch) + v * ch; cg = cg * (1 - ch) + v * 0.88 * ch; cb = cb * (1 - ch) + v * 0.66 * ch; al = (al + (0.96 - al) * ch) * (1 - hole);
        }
      }
      if (al > 0.003) { d[o] = cr; d[o + 1] = cg; d[o + 2] = cb; d[o + 3] = 255 * al; }
    }
    g.putImageData(im, 0, 0);
    return cv;
  }
  // A bubble at the surface: a thin film domed over the liquid. Almost all of it is clear. What shows is a dark
  // refraction rim (heaviest on the side away from the light), a crescent of light focused through the dome onto the
  // far side, the softbox mirrored in the film, a pin of the key light, a bright edge on the lit side, and liquid
  // heaped round the foot, bright toward the light and dark away from it. Worked out per pixel once, then scaled.
  var AC_BS = 128;
  function acBubble() { return acSprite('acid-bubble', acBubbleMake); }
  function acBubbleMake() {
    var pal = AC_RAW, N = AC_BS, cv = document.createElement('canvas'); cv.width = cv.height = N;
    var g = cv.getContext('2d'), im = g.createImageData(N, N), d = im.data, sc = N / 3, Hf = HALF, L = LIGHT, lxy = Math.hypot(L[0], L[1]), lx = L[0] / lxy, ly = L[1] / lxy;
    var sp = pal.spec, gl = pal.glow, dp = pal.deep, pr, pg, pb, al;
    function over(c, A) { if (A <= 0) return; if (A > 1) A = 1; pr = c[0] * A + pr * (1 - A); pg = c[1] * A + pg * (1 - A); pb = c[2] * A + pb * (1 - A); al = A + al * (1 - A); }
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var px = ((x + 0.5) - N / 2) / sc, py = ((y + 0.5) - N / 2) / sc, rho = Math.hypot(px, py), ux = px / (rho || 1), uy = py / (rho || 1), o = (y * N + x) * 4, face = ux * lx + uy * ly;
      pr = pg = pb = al = 0;
      var foot = Math.exp(-Math.pow((rho - 1.1) / 0.1, 2)) * smooth(0.97, 1.03, rho);
      if (rho >= 0.97) {
        over(dp, foot * smooth(0, 0.6, -face) * 0.55);
        over(sp, foot * smooth(0.2, 0.9, face) * 0.16);
      }
      if (rho < 1.02) {
        var rr = Math.min(1, rho) * 0.92, Nz = Math.sqrt(Math.max(0.0001, 1 - rr * rr)), Nx = ux * rr, Ny = uy * rr, edgeIn = 1 - smooth(0.985, 1.02, rho);
        var tilt = 1 - Nz, fre = 0.06 + 0.94 * Math.pow(tilt, 2.6);
                over(dp, Math.pow(smooth(0.62, 0.98, rho), 2.2) * (0.12 + 0.7 * smooth(-0.2, 0.9, -face)) * edgeIn);
        over(gl, Math.pow(Math.max(0, -face), 1.6) * Math.exp(-Math.pow((rho - 0.66) / 0.16, 2)) * 0.8);
        var hh = Nx * Hf[0] + Ny * Hf[1] + Nz * Hf[2], spec = hh > 0.9 ? Math.pow(hh, 260) * 1.7 : 0;
        var win = acWindow(2 * Nz * Nx, 2 * Nz * Ny) * fre * 2.4 + acWindow2(2 * Nz * Nx, 2 * Nz * Ny) * fre * 0.8;
        var edge = Math.exp(-Math.pow((rho - 0.95) / 0.03, 2)) * smooth(0.4, 0.95, face) * 0.22;
        over(sp, Math.min(1, spec + win + edge) * edgeIn);
      }
      if (al > 0) { d[o] = pr / al; d[o + 1] = pg / al; d[o + 2] = pb / al; d[o + 3] = 255 * al; }
    }
    g.putImageData(im, 0, 0);
    return cv;
  }
  // The flash of a bubble bursting: a ring of light that is broken where the film tore first.
  function acPop() { return acSprite('acid-pop', acPopMake); }
  function acPopMake() {
    var pal = AC_RAW, N = 64, cv = document.createElement('canvas'); cv.width = cv.height = N;
    var g = cv.getContext('2d'), im = g.createImageData(N, N), d = im.data, c = pal.foam;
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var px = (x + 0.5 - N / 2) / (N / 2), py = (y + 0.5 - N / 2) / (N / 2), rho = Math.hypot(px, py), an = (Math.atan2(py, px) / TAU + 0.5) * 8;
      var brk = smooth(0.45, 0.7, pvn(an, 3.3, 8) * 0.6 + pvn(an * 2, 7.1, 16) * 0.4), ring = Math.exp(-Math.pow((rho - 0.8) / 0.05, 2)), o = (y * N + x) * 4;
      var A = ring * brk * (1 - smooth(0.92, 1, rho));
      d[o] = c[0]; d[o + 1] = c[1]; d[o + 2] = c[2]; d[o + 3] = 255 * A;
    }
    g.putImageData(im, 0, 0);
    return cv;
  }
  // A droplet flung up when a bubble bursts, or a bead on a creeping drip: a small ball of liquid with its own highlight.
  function acDrop() { return acSprite('acid-drop', acDropMake); }
  function acDropMake() {
    var pal = AC_RAW, N = 32, cv = document.createElement('canvas'); cv.width = cv.height = N;
    var g = cv.getContext('2d'), im = g.createImageData(N, N), d = im.data, L = LIGHT, lxy = Math.hypot(L[0], L[1]), lx = L[0] / lxy, ly = L[1] / lxy, Hf = HALF, sp = pal.spec, gl = pal.drop, dp = pal.deep;
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var px = (x + 0.5 - N / 2) / (N / 2 * 0.86), py = (y + 0.5 - N / 2) / (N / 2 * 0.86), rho = Math.hypot(px, py), o = (y * N + x) * 4;
      if (rho >= 1.04) continue;
      var rr = Math.min(1, rho) * 0.95, Nz = Math.sqrt(1 - rr * rr), Nx = px / (rho || 1) * rr, Ny = py / (rho || 1) * rr, diff = Math.max(0, Nx * L[0] + Ny * L[1] + Nz * L[2]);
      var hh = Nx * Hf[0] + Ny * Hf[1] + Nz * Hf[2], spec = hh > 0.9 ? Math.pow(hh, 120) * 1.4 : 0, tilt = 1 - Nz, win = acWindow(2 * Nz * Nx, 2 * Nz * Ny) * (0.1 + 0.9 * tilt * tilt) * 1.6;
      var k = 0.35 + 0.75 * diff, hl = Math.min(1, spec + win), lee = Math.max(0, -(Nx * lx + Ny * ly)) * tilt * 1.6;
      d[o] = Math.min(255, gl[0] * k * (1 - hl) + sp[0] * hl + gl[0] * lee * 0.3 + dp[0] * (1 - k) * 0.3);
      d[o + 1] = Math.min(255, gl[1] * k * (1 - hl) + sp[1] * hl + gl[1] * lee * 0.3 + dp[1] * (1 - k) * 0.3);
      d[o + 2] = Math.min(255, gl[2] * k * (1 - hl) + sp[2] * hl + gl[2] * lee * 0.3 + dp[2] * (1 - k) * 0.3);
      d[o + 3] = 255 * (1 - smooth(0.94, 1.04, rho)) * 0.96;
    }
    g.putImageData(im, 0, 0);
    return cv;
  }
  // Fumes: a soft, ragged wisp, pale and mostly clear, made once per grade in a handful of shapes.
  var acWispOut = [], acWispKey = [], acWispMk = [];
  function acWisps() {
    for (var k = 0; k < 6; k++) {
      if (!acWispMk[k]) { acWispKey[k] = 'acid-wisp' + k; acWispMk[k] = acWispMaker(k); }
      acWispOut[k] = acSprite(acWispKey[k], acWispMk[k]);
    }
    return acWispOut;
  }
  function acWispMaker(k) { return function () { return acWispOne(k); }; }
  function acWispOne(k) {
    var c = AC_RAW.vap, N = 96, cv = document.createElement('canvas'); cv.width = cv.height = N;
    var g = cv.getContext('2d'), im = g.createImageData(N, N), d = im.data, el = 0.42 + 0.2 * hash(k, 41);
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N * 2 - 1, Y = ((y + 0.5) / N * 2 - 1) / el, rr = Math.hypot(X, Y * 0.6), o = (y * N + x) * 4;
      var w1 = fbm(X * 1.6 + k * 5.1, Y * 1.6 + k * 2.3, 2), n = fbm(X * 3 + w1 * 2.2 + k * 7, Y * 3 + w1 * 2.2, 4);
      var A = smooth(0.38, 0.78, n) * Math.pow(Math.max(0, 1 - rr * rr), 1.4) * (1 - smooth(0.7, 1, Math.abs(Y)));
      d[o] = c[0]; d[o + 1] = c[1]; d[o + 2] = c[2]; d[o + 3] = 255 * A;
    }
    g.putImageData(im, 0, 0);
    return cv;
  }
  // Where the liquid is at angle a (in turns) and a fraction u across it, in token units, drifting with the current.
  function acAt(a, u, out) {
    var e = acEd, i = ((a - Math.floor(a)) * AC_E) | 0, rr = e.ri[i] + u * (e.ro0[i] - e.ri[i]), an = (a - 0.5) * TAU;
    out[0] = Math.cos(an) * rr; out[1] = Math.sin(an) * rr; out[2] = e.ro0[i] - e.ri[i];
  }
  var acP = [0, 0, 0];
  // Fizz. Bubbles come in clusters that form along the contact lines and over the open liquid. Each bubble rises as
  // a small dim dome, swells to the surface, hangs, and bursts in a flash of broken ring, throwing a few droplets when
  // it is big enough to. A cluster is re-rolled when all of its bubbles are gone, so nothing pops in or out.
  var AC_NC = 30, AC_NM = 16;
  function acFizz(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, spr = acBubble(), pop = acPop(), drp = acDrop(), c, m;
    for (c = 0; c < AC_NC; c++) {
      var per = 4 + 5 * hash(c, 201), cyc = t / per + hash(c, 202), cn = Math.floor(cyc), f = cyc - cn, id = c * 41 + cn * 13, zone = hash(id, 203);
      var u0 = zone < 0.28 ? 0.03 + 0.1 * hash(id, 204) : zone < 0.66 ? 0.87 + 0.1 * hash(id, 204) : 0.2 + 0.6 * hash(id, 204);
      var a0 = hash(id, 205) + t * AC_FLOW * 0.92, sp = 0.008 + 0.022 * hash(id, 206), cs = 0.7 + 0.9 * hash(id, 207);
      for (m = 0; m < AC_NM; m++) {
        var mid = id * 17 + m, s0 = 0.45 * hash(mid, 208), du = 0.18 + 0.2 * hash(mid, 209);
        if (f < s0 || f > s0 + du) continue;
        var p = (f - s0) / du, R = (0.0034 + 0.014 * Math.pow(hash(mid, 210), 2.0)) * cs * S0;
        if (R < 0.4) continue;
        acAt(a0, 0.5, acP);
        var ang = (a0 - 0.5) * TAU + (hash(mid, 211) - 0.5) * 2 * sp / 0.45, uu = u0 + (hash(mid, 212) - 0.5) * 2 * sp / acP[2] * 0.7;
        acAt(ang / TAU + 0.5, uu, acP);
        var bx = cx + acP[0] * S0, by = cy + acP[1] * S0;
        if (p < 0.88) {
          var k, al;
          if (p < 0.3) { var q = p / 0.3, e = q * q * (3 - 2 * q); k = 0.2 + 0.8 * e; al = 0.15 + 0.85 * e; }
          else { k = 1 + 0.025 * Math.sin(t * 7 + mid); al = 1; }
          ctx.globalAlpha = al; ctx.drawImage(spr, bx - 1.5 * R * k, by - 1.5 * R * k, 3 * R * k, 3 * R * k);
        } else acBurst(ctx, bx, by, R, (p - 0.88) / 0.12, mid, pop, drp);
      }
    }
    ctx.globalAlpha = 1;
  }
  function acBurst(ctx, bx, by, R, q, id, pop, drp) {
    var e = 1 - (1 - q) * (1 - q), fade = Math.pow(1 - q, 1.3), sz = R * (1.0 + 1.1 * e) * 2.2;
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = fade * 0.95; ctx.drawImage(pop, bx - sz / 2, by - sz / 2, sz, sz);
    ctx.restore();
    var nd = R > 2.2 ? 2 + Math.min(5, Math.floor(R / 2.2)) : 0;
    for (var k = 0; k < nd; k++) {
      var ph = hash(id, 220 + k) * TAU, sv = 0.6 + 1.3 * hash(id, 230 + k), dist = R * (0.5 + sv * e * 1.2), lift = Math.sin(Math.PI * Math.min(1, q * 1.1)) * R * 0.5 * sv, ds = R * (0.1 + 0.1 * hash(id, 240 + k)) * (1 - 0.5 * q);
      if (ds < 0.6) continue;
      ctx.globalAlpha = Math.min(1, fade * 1.6); ctx.drawImage(drp, bx + Math.cos(ph) * dist - ds, by + Math.sin(ph) * dist - lift - ds, ds * 2, ds * 2);
    }
    ctx.globalAlpha = 1;
  }
  // The occasional big bubble: it swells slowly from a dimple, trembles, then bursts with a spray and a ring that
  // runs out over the surface.
  var AC_NB = 4;
  function acBig(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, spr = acBubble(), pop = acPop(), drp = acDrop(), b;
    for (b = 0; b < AC_NB; b++) {
      var per = 8 + 3 * hash(b, 251), cyc = t / per + hash(b, 252), cn = Math.floor(cyc), f = cyc - cn, id = b * 59 + cn * 7;
      if (f < 0.06 || f > 0.62) continue;
      var a = hash(id, 253) + t * AC_FLOW * 0.92, u = 0.36 + 0.28 * hash(id, 254), Rm = (0.016 + 0.014 * hash(id, 255)) * S0;
      acAt(a, u, acP);
      var bx = cx + acP[0] * S0, by = cy + acP[1] * S0, q = (f - 0.06) / 0.4;
      if (q < 1) {
        var sw = q * q * (3 - 2 * q), k = 0.12 + 0.88 * sw, tr = q > 0.7 ? Math.sin(t * 24 + b) * 0.02 * (q - 0.7) / 0.3 : 0;
        ctx.globalAlpha = Math.min(1, 0.3 + sw * 1.4); ctx.drawImage(spr, bx - 1.5 * Rm * k * (1 + tr), by - 1.5 * Rm * k * (1 - tr), 3 * Rm * k * (1 + tr), 3 * Rm * k * (1 - tr));
      } else acBurst(ctx, bx, by, Rm, Math.min(1, (q - 1) / 0.4), id, pop, drp);
    }
    ctx.globalAlpha = 1;
  }
  // The narrow runs end in a bead of acid: a small ball heavier than the run behind it, with its own highlight.
  function acDrips(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, drp = acDrop(), e = acEd, k;
    for (k = AC_NB0; k < AC_NT; k++) {
      var g = e.tg[k], vis = smooth(0.12, 0.4, g) * (1 - smooth(0.9, 1, g));
      if (vis < 0.02) continue;
      var ai = Math.round(e.ta[k] * AC_E) & (AC_E - 1), an = (e.ta[k] - 0.5) * TAU, rt = e.ro[ai] - 0.0015, br = e.tw[k] * (0.7 + 0.3 * g) * 1.25 * S0 * (0.6 + 0.4 * vis);
      ctx.globalAlpha = Math.min(1, vis * 1.4); ctx.drawImage(drp, cx + Math.cos(an) * rt * S0 - br, cy + Math.sin(an) * rt * S0 - br, br * 2, br * 2);
    }
    ctx.globalAlpha = 1;
  }
  // Acid spat off the surface by bursting bubbles: a droplet is flung outward, swells as it passes over, lands on the
  // scorch as a bead with a dark spot under it, and dries away.
  var AC_NS = 14;
  function acSpit(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, drp = acDrop(), e = acEd, d;
    for (d = 0; d < AC_NS; d++) {
      var life = 1.8 + 1.6 * hash(d, 280), cyc = t / life + hash(d, 281), cn = Math.floor(cyc), f = cyc - cn, id = d * 71 + cn * 23;
      if (hash(id, 282) > 0.7) continue;
      var a = hash(id, 283), ai = (a * AC_E) | 0, an = (a - 0.5) * TAU + (hash(id, 284) - 0.5) * 0.7, dx = Math.cos(an), dy = Math.sin(an), r0 = e.ro0[ai] - 0.012;
      var D = Math.min(0.02 + 0.035 * hash(id, 285), 0.545 - r0), bR = (0.0035 + 0.0035 * hash(id, 286)) * S0;
      if (D < 0.008) continue;
      var fl = Math.min(1, f / 0.5), ease = 1 - (1 - fl) * (1 - fl), dist = D * ease * S0, air = f < 0.5 ? Math.sin(Math.PI * fl) : 0;
      var x = cx + dx * r0 * S0 + dx * dist, y = cy + dy * r0 * S0 + dy * dist - air * S0 * 0.012, rr = bR * (1 + 0.7 * air), al;
      if (f < 0.5) al = smooth(0, 0.12, f);
      else { var q = (f - 0.5) / 0.5; rr *= 1 - 0.75 * q * q; al = 1 - q * q; ctx.globalAlpha = al * 0.55; ctx.fillStyle = 'rgba(12,10,5,1)'; ctx.beginPath(); ctx.arc(x + bR * 0.25, y + bR * 0.35, bR * 1.35 * (1 - 0.5 * q), 0, TAU); ctx.fill(); }
      if (rr > 0.45) { ctx.globalAlpha = al; ctx.drawImage(drp, x - rr, y - rr, rr * 2, rr * 2); }
    }
    ctx.globalAlpha = 1;
  }
  // Fumes lift off the liquid, mostly outward over the map and a few inward over the picture; they widen, thin and
  // drift as they go, and are gone before they reach the edge of the canvas.
  var AC_NW = 34;
  function acVapour(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, wsp = acWisps(), w;
    for (w = 0; w < AC_NW; w++) {
      var life = 3.6 + 3 * hash(w, 260), cyc = t / life + hash(w, 261), cn = Math.floor(cyc), f = cyc - cn, id = w * 29 + cn * 5, out = hash(id, 262) < 0.8;
      var a0 = hash(id, 263) + t * AC_FLOW * 0.4, ease = 1 - (1 - f) * (1 - f);
      acAt(a0, out ? 0.7 + 0.3 * hash(id, 264) : 0.1 * hash(id, 264), acP);
      var r0 = Math.hypot(acP[0], acP[1]) || 1, ux = acP[0] / r0, uy = acP[1] / r0, trav = (out ? 0.05 + 0.03 * hash(id, 265) : -0.03 - 0.02 * hash(id, 265)) * ease;
      var side = (qn(w * 1.7 + 3, f * 2.2 + t * 0.15) - 0.5) * 0.07 * f, rc = r0 + trav;
      var x = cx + (ux * rc - uy * side) * S0, y = cy + (uy * rc + ux * side) * S0, half = (0.036 + 0.04 * hash(id, 266)) * (0.6 + 0.9 * f) * S0;
      var al = (out ? 0.7 : 0.3) * smooth(0, 0.18, f) * (1 - smooth(0.4, 1, f)) * (1 - smooth(0.5, 0.548, rc + 0.75 * half / S0));
      if (al < 0.01) continue;
      ctx.save(); ctx.translate(x, y); ctx.rotate(hash(id, 267) * TAU + f * (hash(id, 268) - 0.5) * 1.2);
      ctx.globalAlpha = al; ctx.drawImage(wsp[id % 6], -half, -half, half * 2, half * 2); ctx.restore();
    }
    ctx.globalAlpha = 1;
  }
  // The acid lights the air and the picture's edge: the liquid, shrunk to a few dozen pixels (which blurs it for free).
  var acHaze = { cv: null, q: -2, pk: null };
  var acNow = 0;
  function acGlowDraw(ctx, G, tex) {
    var h = acHaze, ex = AC_EXT * G.bs;
    if (!h.cv) { h.cv = document.createElement('canvas'); h.cv.width = h.cv.height = 40; }
    if (h.q !== acF.q || h.pk !== gradeId()) { var g = h.cv.getContext('2d'); g.clearRect(0, 0, 40, 40); g.imageSmoothingQuality = 'high'; g.drawImage(tex, 0, 0, 40, 40); h.q = acF.q; h.pk = gradeId(); }
    ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.globalAlpha = 0.85; ctx.drawImage(h.cv, G.outW / 2 - ex / 2, G.outH / 2 - ex / 2, ex, ex);
    var cx = G.outW / 2, cy = G.outH / 2, S0 = G.bs, c = acPal().glow, fl = 0.14 + 0.05 * (qn(acNow * 1.6, 3) - 0.5) * 2, gr = ctx.createRadialGradient(cx, cy, S0 * 0.36, cx, cy, S0 * 0.57);
    gr.addColorStop(0, rgba(c, 0)); gr.addColorStop(0.35, rgba(c, fl)); gr.addColorStop(0.62, rgba(c, fl * 0.55)); gr.addColorStop(1, rgba(c, 0));
    ctx.globalAlpha = 1; ctx.fillStyle = gr; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
  }
  function acidFront(ctx, G, t) {
    ctx.save(); ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(graded('acid-scorch', masterLayer('acid-scorch', G.outW, G.outH, acScorchDraw)), 0, 0);
    var tex = acField(t), S0 = G.bs, ex = AC_EXT * S0; acNow = t;
    acGlowDraw(ctx, G, tex);
    ctx.drawImage(tex, G.outW / 2 - ex / 2, G.outH / 2 - ex / 2, ex, ex);
    acDrips(ctx, G, t); acFizz(ctx, G, t); acBig(ctx, G, t); acVapour(ctx, G, t); acSpit(ctx, G, t);
    ctx.restore();
  }
  EXTRA.push({ id: 'acid', name: 'Acid', blurb: 'A ring of bubbling caustic acid, glossy and glowing, eating into the picture\'s edge.',
    key: [120, 220, 40], luma: 0.05, spread: 0.25,
    spec: { anim: true, roundOnly: true, scale: AC_SC, spread: 0.25, rim: 0.11, inset: function (G) { return G.bs * 0.11; }, front: acidFront } });
  /* ---------- Blizzard ----------
     A ring of wind-blown snow round the picture. Real blowing snow is thin: pale, translucent spindrift that
     streams along the wind in long sheets with sharper filaments in them, a fine grit of ice grains streaking past
     faster than the sheets, a few big flakes tumbling slower, and, grown on the picture's edge, rime: a granular
     crust with feathers of frost that point up into the wind. Everything circulates the one way, in the same gusts.
     The sheets and filaments are one small field per frame; grains, flakes and glints are drawn at full size. */
  var BZ_N = 224, BZ_RP = 0.395;
  var bzField = { q: -1, px: null, cv: null, img: null, pal: null, pid: null };
  // Gusts: one shared wobble in the wind's speed, so every layer surges and eases together.
  function bzPhase(t) { return t + 0.3 * Math.sin(t * 0.7) + 0.12 * Math.sin(t * 1.9 + 1); }
  function bzPal() {
    var id = gradeId();
    if (bzField.pid === id) return bzField.pal;
    bzField.pid = id;
    bzField.pal = { pale: tc([172, 204, 246]), white: tc([246, 251, 255]), deep: tc([92, 140, 214]), glow: tc([104, 168, 255]), shade: tc([52, 84, 150]),
      grain: tc([236, 246, 255]), flake: tc([248, 252, 255]) };
    return bzField.pal;
  }
  // Where each field pixel sits round the ring (turns), its radius, how far across the band it is, how fast the wind
  // runs there and how much snow the ring carries there; all of it fixed, so worked out once.
  function bzPixels() {
    var list = [], N = BZ_N;
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, r = Math.hypot(X, Y);
      if (r < 0.34 || r > 0.499) continue;
      var s = (r - 0.34) / 0.16, dr = r - 0.44, env = 0.25 * Math.exp(-dr * dr / 0.0064) + 0.75 * Math.exp(-dr * dr / 0.0009);
      list.push(y * N + x, Math.atan2(Y, X) / TAU + 0.5, s, 0.75 + 0.5 * s, env * (1 - smooth(0.47, 0.498, r)), 0.5 + 0.5 * (X * BZ_LIGHT2[0] + Y * BZ_LIGHT2[1]) / r);
    }
    return new Float32Array(list);
  }
  function bzTexture(t) {
    var F = bzField, N = BZ_N, q = Math.round(t * 30), pal = bzPal();
    if (F.q === q && F.pk === gradeId()) return F.cv;
    if (!F.px) { F.px = bzPixels(); F.cv = document.createElement('canvas'); F.cv.width = F.cv.height = N; F.img = F.cv.getContext('2d').createImageData(N, N); }
    var d = F.img.data, PX = F.px, ph = bzPhase(t), W = pal.white, P = pal.pale, t1 = t * 0.11, t2 = t * 0.09;
    d.fill(0);
    for (var k = 0; k < PX.length; k += 6) {
      var i = PX[k], a = PX[k + 1], s = PX[k + 2], v = PX[k + 3], env = PX[k + 4], lit = 0.62 + 0.38 * PX[k + 5];
      var wx = pvn(a * 4 - ph * 0.18 * v + 3.1, s * 2.2 + t1, 4) - 0.5, wy = pvn(a * 8 - ph * 0.56 * v + 8.7, s * 3.4 - t2 + 5, 8) - 0.5;
      var u1 = a * 8 - ph * 0.88 * v + wx * 1.6, y1 = s * 4.5 + wy * 2.2;
      var n1 = 0.6 * pvn(u1, y1, 8) + 0.28 * pvn(u1 * 2 + 17, y1 * 2 + 9, 16) + 0.12 * pvn(u1 * 4 + 5, y1 * 4 + 3, 32);
      var n2 = pvn(a * 32 - ph * 6 * v + wx * 4, s * 20 + wy * 5 + 11.3, 32), n3 = pvn(a * 64 - ph * 17 * v + wx * 6 + 4.7, s * 34 + wy * 6 + 2.1, 64);
      var sheet = smooth(0.4, 0.8, n1), f1 = 1 - Math.abs(2 * n2 - 1), f2 = 1 - Math.abs(2 * n3 - 1);
      f1 *= f1; f1 *= f1 * f1; f2 *= f2 * f2;
      var rho = env * (0.65 * sheet + 0.6 * f1 * (0.35 + sheet) + 0.22 * f2 * sheet);
      if (rho < 0.004) continue;
      if (rho > 1) rho = 1;
      var o = i * 4, m = Math.sqrt(rho) * lit;
      d[o] = P[0] + (W[0] - P[0]) * m; d[o + 1] = P[1] + (W[1] - P[1]) * m; d[o + 2] = P[2] + (W[2] - P[2]) * m; d[o + 3] = 240 * rho;
    }
    F.cv.getContext('2d').putImageData(F.img, 0, 0); F.q = q; F.pk = gradeId();
    return F.cv;
  }

  // Rime ice, drawn once in the design's own colours (graded() recolours it): a crust and the feathers of frost that
  // grow out of it. Rime builds into the wind, so every feather leans upwind, against the circulation: a long curved
  // spine with barbs down both sides, finer barbs on the long ones. The crust is a lit height field combed along the
  // same lean. Feather edges that face the light are white and the rest a shadowed blue, so it is lit, not outlined.
  var BZ_SPARK = [];
  var BZ_LIGHT2 = [-0.63, -0.78], BZ_BETA = 0.95;
  var bzRamp = [[0, [118, 146, 190]], [0.4, [176, 202, 234]], [0.75, [224, 237, 252]], [1, [255, 255, 255]]];
  function bzRamped(l) {
    for (var q = 1; q < bzRamp.length; q++) if (l <= bzRamp[q][0]) return mix3(bzRamp[q - 1][1], bzRamp[q][1], (l - bzRamp[q - 1][0]) / (bzRamp[q][0] - bzRamp[q - 1][0]));
    return bzRamp[bzRamp.length - 1][1];
  }
  function bzCrust(g, S, w, h) {
    var cx = w / 2, cy = h / 2, Rp = BZ_RP * S, img = g.createImageData(w, h), d = img.data, H = new Float32Array(w * h), x, y, i;
    var cb = Math.cos(BZ_BETA), sb = Math.sin(BZ_BETA), r0 = Math.floor(Rp - 0.03 * S), r1 = Math.ceil(Rp + 0.075 * S);
    for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
      var dx = x + 0.5 - cx, dy = y + 0.5 - cy, r = Math.hypot(dx, dy);
      if (r < r0 || r > r1) continue;
      var nx = dx / r, ny = dy / r, tx = -ny, ty = nx, fx = cb * nx - sb * tx, fy = cb * ny - sb * ty;
      var along = (dx * fx + dy * fy) / S, across = (dx * -fy + dy * fx) / S, e = (r - Rp) / S, th = (Math.atan2(dy, dx) / TAU + 0.5) * 8;
      var fib = vnoise(across * 190 + 7, along * 34) * 0.65 + vnoise(across * 420, along * 70 + 3) * 0.35;
      var E = 0.018 + 0.016 * (pvn(th * 0.5, 0.3, 4) - 0.5) + 0.011 * (pvn(th * 2, 1.7, 16) - 0.5) + 0.012 * (fib - 0.5);
      var base = smooth(-0.012, 0.0, e) * (1 - smooth(E - 0.014, E + 0.004, e));
      H[y * w + x] = base * (0.8 + 0.4 * fib);
    }
    var Dz = 0.016 * S;
    for (y = 1; y < h - 1; y++) for (x = 1; x < w - 1; x++) {
      i = y * w + x; var hv = H[i];
      if (hv <= 0.002) continue;
      var gx = (H[i + 1] - H[i - 1]) * 0.5 * Dz, gy = (H[i + w] - H[i - w]) * 0.5 * Dz, nl = Math.hypot(gx, gy, 1);
      var Nx = -gx / nl, Ny = -gy / nl, Nz = 1 / nl, df = Math.max(0, Nx * LIGHT[0] + Ny * LIGHT[1] + Nz * LIGHT[2]);
      var spec = Math.pow(Math.max(0, Nx * HALF[0] + Ny * HALF[1] + Nz * HALF[2]), 28) * 0.7;
      var lum = clamp(0.2 + 0.62 * df + spec, 0, 1), c = bzRamped(lum), o = i * 4;
      d[o] = c[0]; d[o + 1] = c[1]; d[o + 2] = c[2]; d[o + 3] = 255 * clamp(hv * 1.2, 0, 0.92);
    }
    g.putImageData(img, 0, 0);
  }
  function bzRimLayer(w, h) {
    var S = Math.min(w, h), k = S / 720, cx = w / 2, cy = h / 2, cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), Rp = BZ_RP * S, B = [], i, j;
    for (i = 0; i < 15; i++) B.push([]);
    BZ_SPARK = [];
    function add(x1, y1, x2, y2, wd, lit) {
      var wb = wd < 0.75 ? 0 : wd < 1.3 ? 1 : 2, lb = Math.min(4, Math.floor(clamp(lit, 0, 0.999) * 5));
      B[lb * 3 + wb].push(x1, y1, x2, y2, wd);
    }
    function litOf(nx, ny, dx, dy) { // how much an edge of this direction, at this point of the ring, faces the light
      var face = Math.abs(-dy * BZ_LIGHT2[0] + dx * BZ_LIGHT2[1]), side = Math.max(0, nx * BZ_LIGHT2[0] + ny * BZ_LIGHT2[1]);
      return 0.08 + 0.5 * face + 0.5 * side;
    }
    function feather(th, base, len, inward, seed, ws) {
      var nx = Math.cos(th), ny = Math.sin(th), n = 14, bend = 0.12 + 0.6 * hash(seed, 4), sg = inward ? Math.PI : 0;
      var a0 = th - BZ_BETA * (inward ? -1 : 1) + sg + (hash(seed, 3) - 0.5) * 0.5, px = cx + nx * base, py = cy + ny * base, pts = [[px, py, a0]];
      for (var q = 1; q <= n; q++) { var aq = a0 - bend * Math.pow(q / n, 1.5) * (inward ? -1 : 1); px += Math.cos(aq) * len / n; py += Math.sin(aq) * len / n; pts.push([px, py, aq]); }
      for (q = 1; q <= n; q++) {
        var f = q / n, p0 = pts[q - 1], p1 = pts[q];
        add(p0[0], p0[1], p1[0], p1[1], (2.1 - 1.4 * f) * k * ws, litOf(nx, ny, Math.cos(p1[2]), Math.sin(p1[2])));
        if (q < 3 || q > n - 1) continue;
        for (var sd = -1; sd <= 1; sd += 2) {
          var Lb = len * (0.34 - 0.27 * f) * (0.65 + 0.35 * hash(seed + q, sd + 7)), ab = p1[2] + sd * (0.7 + 0.3 * hash(seed + q, 9)), bdx = Math.cos(ab), bdy = Math.sin(ab);
          var mx = p1[0] + bdx * Lb * 0.55, my = p1[1] + bdy * Lb * 0.55, ab2 = ab - sd * 0.32, ex = mx + Math.cos(ab2) * Lb * 0.45, ey = my + Math.sin(ab2) * Lb * 0.45;
          add(p1[0], p1[1], mx, my, (1.15 - 0.55 * f) * k * ws, litOf(nx, ny, bdx, bdy));
          add(mx, my, ex, ey, (0.8 - 0.4 * f) * k * ws, litOf(nx, ny, Math.cos(ab2), Math.sin(ab2)));
          if (len > 0.032 * S && Lb > 0.009 * S) for (var m = 1; m <= 2; m++) {               // the long barbs carry finer ones
            var bx = p1[0] + bdx * Lb * (0.3 + 0.25 * m), by = p1[1] + bdy * Lb * (0.3 + 0.25 * m), sa = ab - sd * 0.85, sl = Lb * (0.4 - 0.1 * m);
            add(bx, by, bx + Math.cos(sa) * sl, by + Math.sin(sa) * sl, 0.55 * k * ws, litOf(nx, ny, Math.cos(sa), Math.sin(sa)));
          }
          if (sd > 0 && len > 0.04 * S && q === n - 4 && BZ_SPARK.length < 44) BZ_SPARK.push([ex / S, ey / S]);
        }
      }
    }
    bzCrust(g, S, w, h);
    var nF = Math.round(250 * S / 720), clump = function (th) { return 0.3 + 1.5 * smooth(0.2, 0.8, pfbm(th / TAU * 6 + 1, 0.3, 8, 2)); };
    for (i = 0; i < nF; i++) {
      var th = (i + hash(i, 1) * 0.8) / nF * TAU, len = S * (0.024 + 0.07 * Math.pow(hash(i, 2), 1.7)) * clump(th);
      feather(th, Rp + (hash(i, 5) - 0.1) * 0.012 * S, len, false, i * 13, 1);
      if (hash(i, 6) < 0.5) feather(th + 0.012, Rp - 0.004 * S, len * 0.34, true, i * 13 + 5000, 0.6);
    }
    // A fine fur of short fibres along the same lean, at the roots.
    for (i = 0; i < Math.round(1600 * k * k); i++) {
      var tf = hash(i, 21) * TAU, rb = Rp + (hash(i, 22) * 1.1 - 0.15) * 0.03 * S, nx2 = Math.cos(tf), ny2 = Math.sin(tf);
      var inw = hash(i, 23) < 0.25, jj = (hash(i, 24) - 0.5) * 0.8, ang = tf - BZ_BETA + jj + (inw ? Math.PI : 0), fx = Math.cos(ang), fy = Math.sin(ang);
      var L = S * (0.005 + 0.02 * Math.pow(hash(i, 25), 1.8)) * (inw ? 0.5 : 1), x0 = cx + nx2 * rb, y0 = cy + ny2 * rb;
      add(x0, y0, x0 + fx * L, y0 + fy * L, (0.55 + 0.5 * hash(i, 26)) * k, litOf(nx2, ny2, fx, fy) * (0.8 + 0.3 * hash(i, 27)));
    }
    function stroke(arr, dx, dy) {
      g.beginPath();
      for (var q = 0; q < arr.length; q += 5) { g.moveTo(arr[q] + dx, arr[q + 1] + dy); g.lineTo(arr[q + 2] + dx, arr[q + 3] + dy); }
      g.stroke();
    }
    g.lineCap = 'round';
    for (i = 0; i < 15; i++) if (B[i].length) { var wb = i % 3; g.lineWidth = [0.7, 1.15, 1.9][wb] * k; g.strokeStyle = rgba(bzRamped(Math.floor(i / 3) / 4 * 0.95 + 0.02), [0.42, 0.64, 0.84][wb]); stroke(B[i], 0, 0); }
    for (i = 9; i < 15; i++) if (B[i].length) { g.lineWidth = [0.4, 0.6, 0.9][i % 3] * k; g.strokeStyle = 'rgba(255,255,255,.7)'; stroke(B[i], -0.45 * k, -0.55 * k); }       // the lit edges
    // A little bloom as light scatters through the ice, and under it all a cold blue shadow, blurred, so the frost
    // sits on the picture and the map instead of being cut out of them.
    var tmp = document.createElement('canvas'); tmp.width = w; tmp.height = h;
    var tg = tmp.getContext('2d'); tg.filter = 'blur(' + Math.max(1, 3 * k) + 'px)'; tg.drawImage(cv, 0, 0);
    g.save(); g.globalCompositeOperation = 'lighter'; g.globalAlpha = 0.12; g.drawImage(tmp, 0, 0); g.restore();
    var lg = g.createLinearGradient(cx - S * 0.5, cy - S * 0.5, cx + S * 0.5, cy + S * 0.5);
    lg.addColorStop(0.3, 'rgba(70,104,170,0)'); lg.addColorStop(1, 'rgba(70,104,170,.5)');
    g.save(); g.globalCompositeOperation = 'source-atop'; g.fillStyle = lg; g.fillRect(0, 0, w, h); g.restore();
    var sh = document.createElement('canvas'); sh.width = w; sh.height = h; var sg = sh.getContext('2d');
    sg.filter = 'blur(' + Math.max(1, 2.5 * k) + 'px)'; sg.drawImage(cv, 1.6 * k, 2.2 * k); sg.filter = 'none';
    sg.globalCompositeOperation = 'source-in'; sg.fillStyle = 'rgb(30,56,112)'; sg.fillRect(0, 0, w, h);
    var out = document.createElement('canvas'); out.width = w; out.height = h; var og = out.getContext('2d');
    og.globalAlpha = 0.55; og.drawImage(sh, 0, 0); og.globalAlpha = 1; og.drawImage(cv, 0, 0);
    return out;
  }

  // Streamers and grains: the spindrift itself, drawn once as a few layers and then turned round the ring, each at
  // its own speed, so they pass over one another with real parallax. A streamer is a long sliver along the wind,
  // sharp at its head, thinning away to a faint tail, wavering a little and uneven in density along its length. A
  // grain is a speck with a short streak of motion blur. Depth sets how fine, bright and quick a layer is.
  var BZ_LAYERS = [
    { n: 120, kind: 'str', d0: 0.0, d1: 0.3, spd: 0.42, id: 0 }, { n: 260, kind: 'grain', d0: 0.0, d1: 0.35, spd: 0.85, id: 1 },
    { n: 110, kind: 'str', d0: 0.3, d1: 0.65, spd: 0.74, id: 2 }, { n: 70, kind: 'str', d0: 0.65, d1: 1.0, spd: 1.25, id: 3, bloom: 0.6 },
    { n: 180, kind: 'grain', d0: 0.4, d1: 1.0, spd: 2.0, id: 4 },
    { n: 190, kind: 'hair', d0: 0.2, d1: 0.7, spd: 0.62, id: 5 }, { n: 110, kind: 'hair', d0: 0.6, d1: 1.0, spd: 1.5, id: 6 }
  ];
  function bzStreakLayer(w, h, spec) {
    var S = Math.min(w, h), k = S / 720, cx = w / 2, cy = h / 2, cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), i, j, seed = spec.id * 997, hair = spec.kind === 'hair';
    g.lineCap = 'round';
    for (i = 0; i < spec.n; i++) {
      var d = spec.d0 + (spec.d1 - spec.d0) * hash(seed + i, 1), a0 = hash(seed + i, 2) * TAU;
      var r0 = clamp(0.445 + (hash(seed + i, 3) + hash(seed + i, 4) + hash(seed + i, 5) - 1.5) * 0.06, 0.352, 0.494) * S;
      if (spec.kind === 'grain') {
        var tail = (0.02 + 0.09 * d) * (0.6 + 0.8 * hash(seed + i, 6)) * 1.7, wd = Math.max(0.8, S * (0.0011 + 0.0009 * d)), al = 0.3 + 0.6 * d * (0.5 + 0.5 * hash(seed + i, 7));
        g.strokeStyle = 'rgba(238,247,255,' + (al * 0.3).toFixed(3) + ')'; g.lineWidth = wd * 0.8; g.beginPath(); g.arc(cx, cy, r0, a0 - tail, a0); g.stroke();
        g.strokeStyle = 'rgba(244,250,255,' + al.toFixed(3) + ')'; g.lineWidth = wd; g.beginPath(); g.arc(cx, cy, r0, a0 - tail * 0.4, a0); g.stroke();
        continue;
      }
      var L = hair ? 0.1 + 0.6 * Math.pow(hash(seed + i, 6), 1.5) : (0.08 + 0.85 * Math.pow(hash(seed + i, 6), 1.7)) * (0.55 + 0.7 * d);
      var hw = hair ? (0.55 + 0.5 * hash(seed + i, 7)) * k * 0.8 * (0.8 + 0.5 * d) : S * (0.0011 + 0.0055 * Math.pow(d, 1.3) * (0.5 + hash(seed + i, 7))) * 0.5;
      var amp = (0.002 + 0.008 * hash(seed + i, 8)) * S, fq = 2 + 5 * hash(seed + i, 9), ph = hash(seed + i, 10) * TAU;
      var al2 = hair ? 0.3 + 0.55 * hash(seed + i, 11) : 0.26 + 0.62 * d * (0.55 + 0.45 * hash(seed + i, 11));
      var kap = (hash(seed + i, 14) - 0.2) * 0.03 * S;       // most trail outward behind their heads: the wind is a vortex
      var n = 14, tl = a0 - L, outer = [], inner = [], pass, gr;
      hw = Math.max(0.5 * k, hw);
      for (pass = hair ? 1 : 0; pass < 2; pass++) {
        var wm = pass ? 1 : 2.6, am = pass ? 1 : 0.28;
        outer.length = 0; inner.length = 0;
        for (j = 0; j <= n; j++) {
          var sx = j / n, aa = tl + L * sx, ww = hw * wm * Math.pow(Math.sin(Math.PI * 0.5 * Math.min(1, sx / 0.8)), 0.8) * (sx < 0.8 ? 1 : Math.sqrt(Math.max(0, 1 - Math.pow((sx - 0.8) / 0.2, 2))));
          var rr = r0 + amp * Math.sin(fq * aa + ph) + kap * (1 - sx) * L, ca = Math.cos(aa), sa = Math.sin(aa);
          outer.push([cx + ca * (rr + ww), cy + sa * (rr + ww)]); inner.push([cx + ca * (rr - ww), cy + sa * (rr - ww)]);
        }
        gr = g.createConicGradient(tl, cx, cy); var f1 = L / TAU, nst = 7;
        gr.addColorStop(0, 'rgba(240,248,255,0)');
        for (j = 1; j < nst; j++) { var sp = j / nst, lv = Math.pow(sp, 1.4) * (0.55 + 0.45 * pvn(hash(seed + i, 12) * 40 + sp * 5, 0.5, 64)); gr.addColorStop(f1 * sp, 'rgba(240,248,255,' + (al2 * am * lv).toFixed(3) + ')'); }
        gr.addColorStop(f1, 'rgba(250,253,255,' + (al2 * am).toFixed(3) + ')'); gr.addColorStop(Math.min(1, f1 + 0.001), 'rgba(240,248,255,0)'); gr.addColorStop(1, 'rgba(240,248,255,0)');
        g.fillStyle = gr; g.beginPath(); g.moveTo(outer[0][0], outer[0][1]);
        for (j = 1; j <= n; j++) g.lineTo(outer[j][0], outer[j][1]);
        for (j = n; j >= 0; j--) g.lineTo(inner[j][0], inner[j][1]);
        g.closePath(); g.fill();
      }
    }
    if (spec.bloom) {
      var tmp = document.createElement('canvas'); tmp.width = w; tmp.height = h;
      var tg = tmp.getContext('2d'); tg.filter = 'blur(' + Math.max(1, 3.5 * k) + 'px)'; tg.drawImage(cv, 0, 0);
      g.save(); g.globalCompositeOperation = 'lighter'; g.globalAlpha = spec.bloom; g.drawImage(tmp, 0, 0); g.restore();
    }
    return cv;
  }
  function bzLayerDraw(ctx, G, t, spec) {
    var img = graded('blizzard-L' + spec.id, masterLayer('blizzard-L' + spec.id, G.bw, G.bh, function (w, h) { return bzStreakLayer(w, h, spec); }));
    ctx.save(); ctx.translate(G.ox + G.bw / 2, G.oy + G.bh / 2); ctx.rotate(spec.spd * bzPhase(t)); ctx.drawImage(img, -G.bw / 2, -G.bh / 2, G.bw, G.bh); ctx.restore();
  }
  // Big flakes: soft, ragged, never a drawn star. They tumble, so each flashes as a face turns to the light.
  var bzFlakeSpr = {};
  function bzFlakes() {
    var key = gradeId();
    if (bzFlakeSpr[key]) return bzFlakeSpr[key];
    var pal = bzPal(), out = [];
    for (var k = 0; k < 10; k++) {
      var N = 48, cv = document.createElement('canvas'); cv.width = cv.height = N;
      var g = cv.getContext('2d'), im = g.createImageData(N, N), el = 0.6 + 0.35 * hash(k, 3), ca = hash(k, 4) * TAU, cs = Math.cos(ca), sn = Math.sin(ca);
      for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
        var X0 = (x + 0.5) / N * 2 - 1, Y0 = (y + 0.5) / N * 2 - 1, X = (X0 * cs + Y0 * sn), Y = (-X0 * sn + Y0 * cs) / el, rr = Math.hypot(X, Y);
        // A fluffy clump of crystals: a soft body with a raggedly frayed edge, brighter at the middle.
        var edge = 0.72 + 0.28 * (fbm(X * 3.1 + k * 7.1, Y * 3.1 + k * 3.7, 3) * 2 - 1), body = 1 - smooth(edge * 0.35, edge, rr);
        if (body <= 0) continue;
        var core = Math.exp(-rr * rr * 2.2), lit = clamp(0.62 + 0.38 * core - 0.22 * (X0 + Y0) * 0.5 + 0.1 * (vnoise(X * 7 + k, Y * 7) - 0.5), 0, 1), o = (y * N + x) * 4;
        im.data[o] = pal.deep[0] + (pal.flake[0] - pal.deep[0]) * lit; im.data[o + 1] = pal.deep[1] + (pal.flake[1] - pal.deep[1]) * lit; im.data[o + 2] = pal.deep[2] + (pal.flake[2] - pal.deep[2]) * lit;
        im.data[o + 3] = 255 * Math.pow(body, 1.3) * (0.55 + 0.45 * core);
      }
      g.putImageData(im, 0, 0); out.push(cv);
    }
    bzFlakeSpr[key] = out; return out;
  }
  function bzFlakesDraw(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, spr = bzFlakes(), ph = bzPhase(t);
    for (var i = 0; i < 20; i++) {
      var w = 0.55 + 0.8 * hash(i, 31), rr = S0 * (0.385 + 0.1 * hash(i, 32) + 0.012 * Math.sin(t * (0.5 + hash(i, 33)) + i)), a = hash(i, 34) * TAU + w * ph;
      var sz = S0 * (0.009 + 0.011 * hash(i, 35)), tum = Math.cos(t * (0.8 + 1.4 * hash(i, 36)) + i * 2.1), fl = 0.65 + 0.35 * Math.pow(Math.abs(tum), 2);
      ctx.save(); ctx.translate(cx + Math.cos(a) * rr, cy + Math.sin(a) * rr); ctx.rotate(t * (0.5 + hash(i, 37)) + i); ctx.scale(1, 0.35 + 0.65 * Math.abs(tum));
      ctx.globalAlpha = 0.9 * fl; ctx.drawImage(spr[i % spr.length], -sz, -sz, sz * 2, sz * 2);
      ctx.restore();
    }
  }
  // The cold light of it: a blue glow on the picture's edge and a little on the map round the ring.
  function bzGlow(ctx, G, t) {
    var cx = G.outW / 2, cy = G.outH / 2, S0 = G.bs, pal = bzPal(), pulse = 1 + 0.12 * Math.sin(t * 1.3);
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    var g = ctx.createRadialGradient(cx, cy, S0 * 0.3, cx, cy, S0 * BZ_RP);
    g.addColorStop(0, rgba(pal.glow, 0)); g.addColorStop(0.6, rgba(pal.glow, 0.05 * pulse)); g.addColorStop(1, rgba(pal.glow, 0.3 * pulse));
    ctx.save(); ctx.beginPath(); ctx.arc(cx, cy, S0 * BZ_RP, 0, TAU); ctx.clip(); ctx.fillStyle = g; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
    var o = ctx.createRadialGradient(cx, cy, S0 * 0.39, cx, cy, S0 * 0.5);
    o.addColorStop(0, rgba(pal.glow, 0)); o.addColorStop(0.35, rgba(pal.glow, 0.2 * pulse)); o.addColorStop(1, rgba(pal.glow, 0));
    ctx.fillStyle = o; ctx.fillRect(0, 0, G.outW, G.outH);
    ctx.restore();
  }
  // Glitter: the tips of the feathers catch the light for an instant as the wind turns them, a soft flash and a faint cross.
  var bzGlintSpr = {};
  function bzGlintSprite() {
    var key = gradeId();
    if (bzGlintSpr[key]) return bzGlintSpr[key];
    var cv = document.createElement('canvas'); cv.width = cv.height = 64; var g = cv.getContext('2d'), c = bzPal().flake;
    var gr = g.createRadialGradient(32, 32, 0, 32, 32, 14); gr.addColorStop(0, rgba(c, 1)); gr.addColorStop(0.2, rgba(c, 0.55)); gr.addColorStop(1, rgba(c, 0));
    g.fillStyle = gr; g.fillRect(0, 0, 64, 64);
    [[0, 1], [1, 0]].forEach(function (v) {
      var lg = g.createLinearGradient(32 - v[0] * 31, 32 - v[1] * 31, 32 + v[0] * 31, 32 + v[1] * 31);
      lg.addColorStop(0, rgba(c, 0)); lg.addColorStop(0.5, rgba(c, 0.55)); lg.addColorStop(1, rgba(c, 0));
      g.fillStyle = lg; g.fillRect(v[0] ? 1 : 31.4, v[1] ? 1 : 31.4, v[0] ? 62 : 1.2, v[1] ? 62 : 1.2);
    });
    bzGlintSpr[key] = cv; return cv;
  }
  function bzGlints(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, spr = bzGlintSprite();
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    for (var i = 0; i < BZ_SPARK.length; i++) {
      var f = 0.7 + 1.4 * hash(i, 51), on = Math.pow(Math.max(0, Math.sin(t * f + hash(i, 52) * TAU)), 18);
      if (on < 0.02) continue;
      var sz = S0 * (0.016 + 0.014 * hash(i, 53)) * (0.5 + 0.5 * on);
      ctx.globalAlpha = on * 0.85; ctx.drawImage(spr, cx - G.bs / 2 + BZ_SPARK[i][0] * G.bs - sz, cy - G.bs / 2 + BZ_SPARK[i][1] * G.bs - sz, sz * 2, sz * 2);
    }
    ctx.restore();
  }
  function blizzardFront(ctx, G, t) {
    var L = BZ_LAYERS;
    ctx.save(); ctx.imageSmoothingQuality = 'high';
    bzGlow(ctx, G, t);
    bzLayerDraw(ctx, G, t, L[0]); bzLayerDraw(ctx, G, t, L[1]);
    ctx.drawImage(bzTexture(t), 0, 0, G.outW, G.outH);
    bzLayerDraw(ctx, G, t, L[5]); bzLayerDraw(ctx, G, t, L[2]);
    ctx.drawImage(graded('blizzard-rime', masterLayer('blizzard-rime', G.bw, G.bh, bzRimLayer)), G.ox, G.oy);
    bzLayerDraw(ctx, G, t, L[3]); bzLayerDraw(ctx, G, t, L[6]); bzLayerDraw(ctx, G, t, L[4]);
    bzFlakesDraw(ctx, G, t);
    bzGlints(ctx, G, t);
    ctx.restore();
  }
  EXTRA.push({ id: 'blizzard', name: 'Blizzard', blurb: 'A ring of freezing wind and snow, rime ice feathering the picture\'s edge.',
    key: [170, 210, 255], luma: 0.2, spread: 0.5,
    spec: { anim: true, roundOnly: true, rim: 0.105, inset: function (G) { return G.bs * 0.105; }, front: blizzardFront } });
  /* ---------- Iron chain ----------
     A heavy forged chain laid flat round the token: links alternate between lying face-on (an oval loop with a
     real hole) and standing on edge (seen from above as a rounded bar that rides over the neighbouring loops).
     The geometry is solved, not drawn: every link is a round-section wire bent into an oval, so the height at a
     point is the top of whichever wire is highest there, and the shader's normals, shadows and occlusion do the
     rest. Units below are wire diameters, with the wire radius 0.5. */
  var CHAIN_N = 26;                 // links round a round token (even, so face-on and edge-on alternate with no seam)
  var CHAIN_PITCH = 3.3;            // centre to centre of neighbours, which is also a link's inner length
  var CHAIN_WIN = 1.5;              // inner width of a link
  var CHAIN_RHO = (CHAIN_WIN + 1) / 2;          // radius of the wire's centre line round the end of a link
  var CHAIN_D = Math.PI / (CHAIN_N * CHAIN_PITCH + 11);   // wire diameter as a fraction of the token size, so the links fit the ring exactly
  var CHAIN_W = CHAIN_WIN + 2;      // width of a face-on link, which is the width of the ring
  var CHAIN_ZM = 0.5 + CHAIN_RHO;   // height of the arc centres of an edge-on link
  var CHAIN_ZTOP = 0.5 + 2 * CHAIN_RHO + 1;     // top of an edge-on link, the highest thing in the ring
  var CHAIN_LIFT = 0.9;             // face-on links rest on the table by their middle and on their neighbours by their ends
  var chainSets = {};
  var chainHW = 0.5, chainHH = 0.5; // half the frame's width and height in token sizes, learned when the shader starts a pass

  // The link table for one shape: centre, the two directions the link's arms point (opposite for a straight link,
  // at right angles for the bent link that carries a corner of a square or a rectangle), the inner length and the
  // half-length of the spine. A rectangle gets an even number of links to a side, so every corner carries a face-on link.
  function chainLinks(isRound) {
    var key = isRound ? 'r' : chainHW.toFixed(5) + ',' + chainHH.toFixed(5);
    if (chainSets[key]) return chainSets[key];
    var D = CHAIN_D, i, L = { round: isRound, n: CHAIN_N, cx: [], cy: [], ux: [], uy: [], vx: [], vy: [], li: 0, hl: 0, ax: 0, ay: 0, mx: 0, my: 0, px: 0, py: 0 };
    if (isRound) {
      var Rc = 0.5 - CHAIN_W / 2 * D, chord = 2 * Rc * Math.sin(Math.PI / CHAIN_N);
      L.li = chord / D; L.ax = Rc;
      for (i = 0; i < CHAIN_N; i++) {
        var th = -Math.PI / 2 + TAU * i / CHAIN_N, tx = -Math.sin(th), ty = Math.cos(th);
        L.cx.push(Rc * Math.cos(th) / D); L.cy.push(Rc * Math.sin(th) / D); L.ux.push(tx); L.uy.push(ty); L.vx.push(-tx); L.vy.push(-ty);
      }
    } else {
      var ax = chainHW - CHAIN_W / 2 * D, ay = chainHH - CHAIN_W / 2 * D, p0 = CHAIN_PITCH * D;
      var mx = 2 * Math.max(1, Math.round(ax / p0)), my = 2 * Math.max(1, Math.round(ay / p0)), px = 2 * ax / mx, py = 2 * ay / my;
      var V = [[-ax, -ay], [ax, -ay], [ax, ay], [-ax, ay]], DR = [[1, 0], [0, 1], [-1, 0], [0, -1]], cnt = [mx, my, mx, my], pit = [px, py, px, py], j, m;
      L.n = 2 * (mx + my); L.ax = ax; L.ay = ay; L.mx = mx; L.my = my; L.px = px; L.py = py; L.li = Math.max(px, py) / D;
      for (i = 0, j = 0; j < 4; j++) for (m = 0; m < cnt[j]; m++, i++) {
        var dj = DR[j], dp = DR[(j + 3) % 4];
        L.cx.push((V[j][0] + m * pit[j] * dj[0]) / D); L.cy.push((V[j][1] + m * pit[j] * dj[1]) / D);
        if (m === 0) { L.ux.push(-dp[0]); L.uy.push(-dp[1]); L.vx.push(dj[0]); L.vy.push(dj[1]); }
        else { L.ux.push(dj[0]); L.uy.push(dj[1]); L.vx.push(-dj[0]); L.vy.push(-dj[1]); }
      }
    }
    L.hl = (L.li + 1) / 2 - CHAIN_RHO;
    return (chainSets[key] = L);
  }
  var CHS = { d: 0, t: 0 };
  function chainSeg(px, py, ax, ay, bx, by) {
    var dx = bx - ax, dy = by - ay, t = ((px - ax) * dx + (py - ay) * dy) / (dx * dx + dy * dy);
    t = t < 0 ? 0 : t > 1 ? 1 : t;
    var ex = px - ax - t * dx, ey = py - ay - t * dy;
    CHS.d = Math.sqrt(ex * ex + ey * ey); CHS.t = t;
  }
  // What the topmost wire at a point is: its height z, whether its link lies flat (0) or stands (1), which link, the
  // wire's own coordinates (q across it, lx along the link), and how close the point is to where the link rubs its neighbour.
  var CHR = { z: 0, kind: -1, link: 0, q: 0, lx: 0, ly: 0, cw: 0, T: 0, cov: 1, under: false };
  var chainPxd = 0.043, chainLastX = 0, chainLastY = 9; // one pixel in wire diameters, learned from the shader's raster order, for edge coverage
  function chainEval(X, Y, L) {
    var D = CHAIN_D, px = X / D, py = Y / D, n = L.n, k0 = 0, hl = L.hl, best = -1, second = -1, bk = -1, bi = 0, bq = 0, blx = 0, bly = 0, bcov = 1, k, i, pxd = chainPxd;
    if (L.round) {
      k0 = Math.floor((Math.atan2(Y, X) + Math.PI / 2) / (TAU / n));
    } else {                                                     // which link the point's nearest side starts it at
      var ax = L.ax, ay = L.ay, mx = L.mx, my = L.my;
      if (Math.abs(ax - Math.abs(X)) < Math.abs(ay - Math.abs(Y))) k0 = X > 0 ? mx + Math.floor((clamp(Y, -ay, ay) + ay) / L.py) : 2 * mx + my + Math.floor((ay - clamp(Y, -ay, ay)) / L.py);
      else k0 = Y < 0 ? Math.floor((clamp(X, -ax, ax) + ax) / L.px) : mx + my + Math.floor((ax - clamp(X, -ax, ax)) / L.px);
    }
    for (k = k0 - 1; k <= k0 + 2; k++) {
      i = ((k % n) + n) % n;
      var cx = L.cx[i], cy = L.cy[i], ux = L.ux[i], uy = L.uy[i], z = -1, q = 0, cov = 1, lx, ly;
      lx = (px - cx) * ux + (py - cy) * uy; ly = -(px - cx) * uy + (py - cy) * ux;
      if (i % 2 === 0) {                                           // lies face-on: a wire bent round the spine
        chainSeg(px, py, cx + hl * ux, cy + hl * uy, cx, cy); var d1 = CHS.d;
        chainSeg(px, py, cx, cy, cx + hl * L.vx[i], cy + hl * L.vy[i]);
        var d2 = CHS.d, dd = Math.min(d1, d2);
        if (ux * L.vx[i] + uy * L.vy[i] > -0.9) { var hh = Math.max(1.2 - Math.abs(d1 - d2), 0) / 1.2; dd -= hh * hh * 0.3; }   // the bent link at a corner rounds the inside of its bend
        q = dd - CHAIN_RHO;
        cov = clamp(0.5 + (0.5 - Math.abs(q)) / pxd, 0, 1);       // half a pixel of reach beyond the wire's edge, so its outline is smooth
        if (cov > 0) z = 0.5 + CHAIN_LIFT * smooth(0.4, 2.2, Math.hypot(px - cx, py - cy)) + Math.sqrt(Math.max(0, 0.25 - q * q));
      } else {                                                     // stands on edge: a bar whose ends curve down out of sight
        var al = Math.abs(ly);
        if (al < 0.5 + pxd) {
          var e = Math.sqrt(Math.max(0, 0.25 - ly * ly)), dx = Math.max(0, Math.abs(lx) - hl), Dm = CHAIN_RHO + e;
          cov = Math.min(clamp(0.5 + (0.5 - al) / pxd, 0, 1), clamp(0.5 + (Dm - dx) / pxd, 0, 1));
          if (cov > 0) { z = CHAIN_ZM + Math.sqrt(Dm * Dm - dx * dx); q = ly; }
        }
      }
      if (z > best) { second = best; best = z; bk = i % 2; bi = i; bq = q; blx = lx; bly = ly; bcov = cov; } else if (z > second) second = z;
    }
    CHR.cov = bcov; CHR.under = second >= 0;
    CHR.z = best; CHR.kind = best < 0 ? -1 : bk; CHR.link = bi; CHR.q = bq; CHR.lx = blx; CHR.ly = bly;
    if (best >= 0) {
      var dcx = Math.abs(blx) - L.li / 2;
      CHR.cw = Math.exp(-(dcx * dcx + (bk ? 0 : bly * bly)) / 0.45);
      if (bk) CHR.T = blx;
      else {                                                       // distance round the loop of the wire, so grain and the weld follow the wire
        var ex = Math.abs(blx) - hl, rho = CHAIN_RHO, half = Math.PI * rho;
        if (ex <= 0) CHR.T = bly >= 0 ? blx + hl : 2 * hl + half + hl - blx;
        else if (blx > 0) CHR.T = 2 * hl + rho * (Math.PI / 2 - Math.atan2(bly, ex));
        else CHR.T = 4 * hl + half + rho * (Math.PI / 2 - Math.atan2(-bly, ex));
      }
    }
    return CHR;
  }
  // Hammer facets: shallow flats beaten into the iron, each tilted its own way and rounded off where they meet.
  var chainFacetId = 0;
  function chainFacet(X, Y) {
    var fx = X * 34 + 3.1, fy = Y * 34 - 1.7, v = voronoi(fx, fy), id = v.id;
    chainFacetId = id;
    var ta = (id - 0.5) * 2, tb = (fract(id * 37.7) - 0.5) * 2;
    return ((fx - v.cx) * ta + (fy - v.cy) * tb) * smooth(0.0, 0.22, v.f2 - v.f1) * smooth(0.2, 0.55, vnoise(X * 6 + 11, Y * 6));
  }
  // The forge weld: a faint collar round the wire where the bar was closed, on the top side of some links and out of sight on others.
  function chainWeld(r, L) {
    var lk = r.link, d;
    if (r.kind) { d = r.lx - (lat(lk, 71) < 0.5 ? -1 : 1) * L.hl * (0.3 + 0.4 * lat(lk, 73)); if (lat(lk, 79) < 0.4) return 9; }
    else d = wrapD(r.T - (0.25 + 0.5 * lat(lk, 73)), 4 * L.hl + 2 * Math.PI * CHAIN_RHO);
    return Math.abs(d) < 0.3 ? d : 9;
  }
  var CHAIN_MAT = {
    width: CHAIN_W * CHAIN_D, depth: 0.8, shadow: 0, ao: true, selfShadow: true,
    height: function (u, X, Y, A, r) {
      if (Y < chainLastY) { chainHW = r - X; chainHH = r - Y; }          // a pass starts at the top-left pixel, whose place gives the frame's half sizes
      if (Y === chainLastY && X > chainLastX && X - chainLastX < 0.01) chainPxd = (X - chainLastX) / CHAIN_D;
      chainLastX = X; chainLastY = Y;
      var L = chainLinks(this.isRound), ev = chainEval(X, Y, L);
      if (ev.kind < 0) return NaN;
      var z = ev.z + chainFacet(X, Y) * 0.2 + (fbm(X * 11 + 5, Y * 11, 3) - 0.5) * 0.2;
      z += (vnoise(X * 120, Y * 120) - 0.5) * 0.05 * bl(120) + (fbm(X * 45 + 2, Y * 45, 3) - 0.5) * 0.08;  // hammered, pitted and scaled
      var pv = voronoi(X * 95 + 9, Y * 95), pr = 0.08 + 0.12 * fract(pv.id * 91.7);
      if (pv.id < 0.22) z -= (1 - smooth(pr * 0.3, pr, pv.f1)) * 0.05 * bl(95);                       // pits where it has corroded
      var wd = chainWeld(ev, L);
      if (wd < 9) z += 0.035 * bead(-0.3, 0.3, wd);
      return z / CHAIN_ZTOP;
    },
    color: function (s) {
      var L = chainLinks(this.isRound), r = chainEval(s.X, s.Y, L), N = s.N, lk = r.link, X = s.X, Y = s.Y;
      var v1 = lat(lk, 31), v2 = lat(lk, 47), aq = Math.abs(r.q) * 2, crest = 1 - smooth(0.2, 0.95, aq);
      var occ = clamp(s.occ * 6, 0, 1), k;
      // Iron is a metal, so it is lit by what it reflects: a dim overhead sky, a key light, and a dark floor.
      var Rx = 2 * N[2] * N[0], Ry = 2 * N[2] * N[1], Rz = 2 * N[2] * N[2] - 1;
      var rough = 0.6 + 0.8 * vnoise(X * 70 + lk, Y * 70) * bl(70) + 0.3 * fbm(X * 24, Y * 24, 2);
      var wear = clamp(smooth(0.62, 0.8, fbm(X * 18 + 3, Y * 18, 3)) * (0.25 + 0.75 * crest) + r.cw * 0.8 * (0.4 + 0.6 * crest), 0, 1);
      var dome = (0.03 + 0.18 * Math.pow(smooth(-0.1, 0.95, Rz), 1.4)) * (1 - 0.8 * occ);
      var key = specular(N, 24 + 40 * wear) * (0.45 + 1.1 * wear) * s.lit * rough;
      var fill = Math.pow(Math.max(0, -Rx * 0.6 - Ry * 0.55 - Rz * 0.2 + 0.2), 3) * 0.14 * (1 - occ);
      var sheen = specular(N, 6) * 0.16 * s.lit + (0.2 + 0.8 * s.diff) * 0.1 * (1 - 0.6 * occ);
      var fe = [0.6 + 0.1 * v1, 0.65 + 0.08 * v1, 0.76 + 0.06 * v1];
      var grain = (vnoise(r.T * 1.7 + lk * 7.3, r.q * 6 + lk) - 0.5) * bl(190) * 0.5 + (vnoise(X * 120, Y * 120) - 0.5) * 0.25 * bl(120);
      chainFacet(X, Y);
      var tone = 0.88 + 0.24 * chainFacetId, polish = smooth(0.7, 0.95, fract(chainFacetId * 53.3 + v1 * 7)) * (0.3 + 0.7 * wear + r.cw * 0.5);
      var tight = specular(N, 90) * (0.3 + 1.2 * polish) * s.lit * rough;
      var e = ((dome * rough + key + fill + sheen) * tone + tight) * (1 + grain);
      var out = [255 * fe[0] * e, 255 * fe[1] * e, 255 * fe[2] * e];
      // Forge scale: blue-black and matte, in fine soft patches that follow the wire.
      var sc = smooth(0.45, 0.8, fbm(r.T * 1.6 + lk * 11, r.q * 6 + v1 * 9, 3)) * (1 - wear * 0.8) * 0.6;
      var dl = (0.08 + 0.9 * s.diff) * (1 - 0.6 * occ) * (1 + grain);
      for (k = 0; k < 3; k++) out[k] = out[k] * (1 - sc) + [38, 42, 52][k] * dl * sc;
      // A little rust, in the joints and the creases, ragged at its edges and thinning to freckles; some links more than others.
      var rn = fbm(X * 30 + 7, Y * 30, 3) * 0.5 + vnoise(X * 100, Y * 100) * 0.5 * bl(100);
      var rv = rn * 0.55 + occ * 0.55 + r.cw * 0.5 * (0.5 + v2) + v2 * 0.12 - 0.26 + (fbm(X * 70 + 3, Y * 70, 3) - 0.5) * 0.5 * bl(70);
      var rust = smooth(0.2, 0.5, rv) * 0.85 * (1 - wear * 0.6);
      var rc = mix3([84, 42, 22], [150, 86, 40], smooth(0.3, 0.8, fbm(X * 44, Y * 44, 2)) * (1 - rust * 0.5)), rl = (0.18 + 0.8 * s.diff) * (1 - 0.5 * occ) * (0.7 + 0.6 * vnoise(X * 150, Y * 150) * bl(150));
      for (k = 0; k < 3; k++) out[k] = out[k] * (1 - rust) + rc[k] * rl * rust;
      var wd = chainWeld(r, L);
      if (wd < 9) { var wl = bead(-0.05, 0.05, wd) * 0.18; for (k = 0; k < 3; k++) out[k] *= 1 - wl; }
      out.push(r.under ? 1 : Math.min(1, r.cov / 0.6));          // the shader halves the alpha of a pixel on the outline, so ask for more
      return out;
    }
  };

  // The picture is a disc lying under the chain, so its cut edge shows through the loops. The chain shades the picture and the map
  // through them, and the layer below is drawn over the picture with the chain itself cut out of it.
  function chainInset(bs) { return CHAIN_W * CHAIN_D * bs * 0.55; }
  function chainUnderDraw(w, h, isRound) {
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), bs = Math.min(w, h), rim = rimLayer('chain', w, h, isRound, CHAIN_MAT), d = CHAIN_D * bs;
    var sil = document.createElement('canvas'); sil.width = w; sil.height = h;
    var sg = sil.getContext('2d'); sg.drawImage(rim, 0, 0); sg.globalCompositeOperation = 'source-in'; sg.fillStyle = '#000'; sg.fillRect(0, 0, w, h);
    // the picture's cut edge: a fine dark line with a faint light one inside it
    shapePath(g, w, h, chainInset(bs), isRound); g.lineWidth = Math.max(1, bs * 0.004); g.strokeStyle = 'rgba(0,0,0,.6)'; g.stroke();
    shapePath(g, w, h, chainInset(bs) + g.lineWidth * 1.1, isRound); g.lineWidth = Math.max(0.6, bs * 0.002); g.strokeStyle = 'rgba(255,255,255,.12)'; g.stroke();
    // a tight contact shadow and a looser cast one, both falling away from the light
    g.filter = 'blur(' + d * 0.22 + 'px)'; g.globalAlpha = 0.5; g.drawImage(sil, d * 0.14, d * 0.2);
    g.filter = 'blur(' + d * 0.65 + 'px)'; g.globalAlpha = 0.34; g.drawImage(sil, d * 0.75, d * 0.93);
    g.filter = 'none'; g.globalAlpha = 1; g.globalCompositeOperation = 'destination-out'; g.drawImage(rim, 0, 0);
    return cv;
  }
  function chainBack(ctx, G) {
    ctx.save(); ctx.filter = 'blur(' + Math.max(1, G.bs * 0.01) + 'px)'; ctx.fillStyle = 'rgba(0,0,0,.45)';
    ctx.translate(G.ox + G.bs * 0.004, G.oy + G.bs * 0.008); shapePath(ctx, G.bw, G.bh, chainInset(G.bs), G.isRound); ctx.fill(); ctx.restore();
  }
  function chainFront(ctx, G) {
    ctx.drawImage(masterLayer('chain-under|' + G.isRound, G.bw, G.bh, function (w, h) { return chainUnderDraw(w, h, G.isRound); }), G.ox, G.oy);
  }

  EXTRA.push({ id: 'chain', name: 'Iron chain', blurb: 'A heavy forged chain laid round the token, links alternating flat and on edge, the picture showing through the loops.',
    key: [70, 70, 74], luma: 0.4, spec: { mat: CHAIN_MAT, inset: function (G) { return chainInset(G.bs); }, back: chainBack, front: chainFront } });

  /* ---------- Divine: a gilded halo ----------
     The halo round a saint in an old panel painting: burnished gold leaf laid over raised gesso and tooled by hand.
     A row of pearls round the outside, a field of incised rays with punched rosettes between them, a fine incised
     line and a fillet at the inside. The leaf is a mirror, so it shows the room: bright where it turns to the window,
     dark where it faces the floor, and every punch catches the light as a spark. Where hands have rubbed it, the
     leaf has worn through to the red clay under it. Behind it a soft glory of light turns slowly, a sheen passes
     round the gold, and motes of light drift up off it. Calm light: no lightning, no crystals. */
  var DIVINE_W = 0.07;
  function divinePearl(A, u, per) { // the pearl over this point: 0 outside it, up to 1 at its top
    var step = per / Math.round(per / 0.0132), j = Math.round(A / step), da = wrapD(A - j * step, per);
    var du = (u - 0.115) * DIVINE_W, rp = 0.0052 * (0.92 + 0.16 * lat(((j % 997) + 997) % 997, 21)), d2 = (da * da + du * du) / (rp * rp);
    return d2 < 1 ? Math.sqrt(1 - d2) : 0;
  }
  function divineRosette(A, u, per) { // the depth of a punched rosette here: a centre punch ringed by six petals
    var step = per / Math.round(per / 0.052), j = Math.round(A / step), da = wrapD(A - j * step, per), du = (u - 0.56) * DIVINE_W;
    var dd = Math.hypot(da, du), best = 0;
    if (dd > 0.0062) return 0;
    var c2 = dd * dd / (0.0012 * 0.0012); if (c2 < 1) best = 1 - c2;
    var ang = Math.atan2(du, da), sect = Math.round(ang / (TAU / 6)) * (TAU / 6);
    var px = Math.cos(sect) * 0.0036, py = Math.sin(sect) * 0.0036, e2 = (Math.pow(da - px, 2) + Math.pow(du - py, 2)) / (0.0011 * 0.0011);
    if (e2 < 1) best = Math.max(best, 1 - e2);
    return best;
  }
  function divineDots(A, u, uc, per) { // a row of single punches
    var step = per / Math.round(per / 0.0058), j = Math.round(A / step), da = wrapD(A - j * step, per), du = (u - uc) * DIVINE_W;
    var d2 = (da * da + du * du) / (0.00105 * 0.00105);
    return d2 < 1 ? 1 - d2 : 0;
  }
  function divineRayCut(A, u, per) { // incised rays across the field, stopping short of each rosette
    var step = per / Math.round(per / 0.0074), j = Math.round(A / step), da = wrapD(A - j * step, per);
    var stepR = per / Math.round(per / 0.052), jr = Math.round(A / stepR), dr = Math.hypot(wrapD(A - jr * stepR, per), (u - 0.56) * DIVINE_W);
    var len = smooth(0.34, 0.38, u) * (1 - smooth(0.76, 0.8, u)) * smooth(0.0072, 0.0084, dr);
    return (1 - smooth(0.0003, 0.0009, Math.abs(da))) * len;
  }
  var DIVINE_MAT = {
    width: DIVINE_W, depth: 0.42, ao: true, selfShadow: true, shadow: 0.7,
    height: function (u, X, Y, A) {
      var per = perOf(this);
      // The profile: a rounded lip, the channel the pearls sit in, a fillet, the domed field, an incised line, a
      // second fillet and a soft fall to the picture.
      var h = 0.32 + 0.16 * smooth(0, 0.03, u) - 0.04 * smooth(0.16, 0.2, u) + 0.2 * smooth(0.2, 0.23, u) * (1 - smooth(0.24, 0.27, u));
      h += 0.18 * smooth(0.26, 0.3, u) * (1 - smooth(0.82, 0.86, u)) * (0.85 + 0.15 * Math.sin(Math.PI * clamp((u - 0.26) / 0.6, 0, 1)));
      h += 0.16 * smooth(0.86, 0.88, u) * (1 - smooth(0.9, 0.93, u)) + 0.1 * smooth(0.88, 0.9, u) - 0.22 * smooth(0.92, 1, u);
      h += (fbm(X * 16 + 7, Y * 16, 3) - 0.5) * 0.035;                                   // gesso is laid by hand, never quite flat
      var pe = divinePearl(A, u, per); if (pe > 0) h = Math.max(h, 0.42 + 0.34 * pe);
      h -= 0.07 * divineRosette(A, u, per) + 0.06 * divineDots(A, u, 0.32, per) + 0.06 * divineDots(A, u, 0.8, per);
      h -= 0.035 * divineRayCut(A, u, per);
      // Leaf is laid in squares that overlap a little, leaving faint straight seams across the gold.
      var gx = Math.abs(fract((X + Y * 0.12) * 9.1) - 0.5), gy = Math.abs(fract((Y - X * 0.12) * 9.1 + 0.3) - 0.5);
      h += 0.008 * (smooth(0.49, 0.497, Math.max(gx, gy)));
      return h;
    },
    color: function (s) {
      var N = s.N, Rx = 2 * N[2] * N[0], Ry = 2 * N[2] * N[1], Rz = 2 * N[2] * N[2] - 1;
      // The room the gold mirrors: a window high on the left, a warm ceiling, dark floor and walls.
      var kx = Rx + 0.32, ky = Ry + 0.38, win = Math.exp(-(kx * kx + ky * ky) / 0.05) * 1.1 + Math.exp(-(kx * kx + ky * ky) / 0.3) * 0.35;
      var key = Math.pow(Math.max(0, Rx * LIGHT[0] + Ry * LIGHT[1] + Rz * LIGHT[2]), 28) * 0.9;
      var room = 0.24 + 0.55 * smooth(-0.6, 0.8, -Ry - Rx * 0.4) + 0.12 * smooth(0.6, 1, Rz);
      var env = (win + key) * s.lit + room * (0.55 + 0.45 * s.lit);
      var occ = clamp(s.occ * 6, 0, 1), grain = (vnoise(s.X * 420, s.Y * 420) - 0.5) * bl(420);
      var gold = [255, 198, 98], e = env * (1 - occ * 0.55) * (1 + grain * 0.12);
      var out = [gold[0] * (0.07 + e * 0.92), gold[1] * (0.06 + e * 0.86), gold[2] * (0.04 + e * 0.7)];
      for (var k = 0; k < 3; k++) out[k] += Math.max(0, e - 1) * 120;                          // the brightest reflections go white
      // Rubbed through to the red bole on the high points, in patches, the way hands wear it.
      var hi = smooth(0.62, 0.8, s.h), wn = fbm(s.X * 11 + 2, s.Y * 11, 4) * 0.7 + vnoise(s.X * 70, s.Y * 70) * 0.3 * bl(70);
      var worn = smooth(0.66, 0.74, wn * 0.8 + hi * 0.3) * hi * 0.7;
      var bole = mix3([128, 52, 36], [96, 40, 30], vnoise(s.X * 90, s.Y * 90)), bl2 = 0.35 + s.diff * 0.75;
      for (k = 0; k < 3; k++) out[k] = out[k] * (1 - worn) + bole[k] * bl2 * worn;
      // Old varnish and dust settle in the punches and the cuts.
      var grime = occ * 0.45 + smooth(0.3, 1.2, s.curv * 6) * 0.25;
      out[0] = out[0] * (1 - grime) + 46 * grime; out[1] = out[1] * (1 - grime) + 32 * grime; out[2] = out[2] * (1 - grime) + 14 * grime;
      return out;
    }
  };

  // The glory: many fine rays of different lengths, drawn small and scaled up so they come out as soft light.
  var DIVINE_N = 220, divineTex = { cv: null, q: -1, k: '' };
  function divineGlory(t, pale, gold) {
    var q = Math.round(t * 30), k = gradeId(), D = divineTex, N = DIVINE_N;
    if (D.q === q && D.k === k) return D.cv;
    if (!D.cv) { D.cv = document.createElement('canvas'); D.cv.width = D.cv.height = N; }
    var g = D.cv.getContext('2d'), c = N / 2, R0 = N * 0.425, NR = 84;
    g.clearRect(0, 0, N, N); g.globalCompositeOperation = 'lighter';
    var halo = g.createRadialGradient(c, c, R0 * 0.98, c, c, N * 0.5);
    halo.addColorStop(0, rgba(pale, 0.95)); halo.addColorStop(0.12, rgba(pale, 0.55)); halo.addColorStop(0.4, rgba(gold, 0.24)); halo.addColorStop(1, rgba(gold, 0));
    g.fillStyle = halo; g.beginPath(); g.arc(c, c, N * 0.5, 0, TAU); g.arc(c, c, R0 * 0.98, 0, TAU, true); g.fill();
    for (var i = 0; i < NR; i++) {
      var a = t * 0.035 + (i + 0.5 * hash(i, 2)) / NR * TAU, br = 0.55 + 0.45 * Math.sin(t * (0.5 + 0.4 * hash(i, 5)) + i * 2.4);
      var L = N * (0.035 + 0.075 * Math.pow(hash(i, 3), 1.3)) * (0.75 + 0.25 * br), w0 = N * (0.004 + 0.005 * hash(i, 4));
      var ca = Math.cos(a), sa = Math.sin(a), nx = -sa, ny = ca;
      var gr = g.createLinearGradient(c + ca * R0, c + sa * R0, c + ca * (R0 + L), c + sa * (R0 + L));
      gr.addColorStop(0, rgba(pale, 0.9 * br)); gr.addColorStop(0.35, rgba(gold, 0.45 * br)); gr.addColorStop(1, rgba(gold, 0));
      g.fillStyle = gr; g.beginPath();
      g.moveTo(c + ca * R0 + nx * w0, c + sa * R0 + ny * w0); g.lineTo(c + ca * (R0 + L), c + sa * (R0 + L)); g.lineTo(c + ca * R0 - nx * w0, c + sa * R0 - ny * w0);
      g.closePath(); g.fill();
    }
    D.q = q; D.k = k; return D.cv;
  }
  // A sheen passing slowly round the gold: the gold's own light added back through a soft window that turns.
  var divineSheenCv = null;
  function divineBack(ctx, G, t) {
    var gold = tc([255, 206, 120]), pale = tc([255, 246, 220]);
    ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(divineGlory(t, pale, gold), 0, 0, G.outW, G.outH); ctx.restore();
  }
  function divineFront(ctx, G, t) {
    var S1 = Math.min(G.outW, G.outH), cx = G.outW / 2, cy = G.outH / 2, R1 = G.bs / 2, gold = tc([255, 206, 120]), pale = tc([255, 246, 220]);
    var rim = rimLayer('divine', G.bw, G.bh, true, DIVINE_MAT);
    rim = graded('rim|divine', rim);
    if (!divineSheenCv) divineSheenCv = document.createElement('canvas');
    var sc = divineSheenCv; if (sc.width !== G.outW || sc.height !== G.outH) { sc.width = G.outW; sc.height = G.outH; }
    var x = sc.getContext('2d'); x.clearRect(0, 0, sc.width, sc.height); x.globalCompositeOperation = 'source-over';
    x.drawImage(rim, G.ox, G.oy);
    var a0 = t * 0.32 - 2.2, cg = x.createConicGradient(a0, cx, cy);
    cg.addColorStop(0, 'rgba(0,0,0,0)'); cg.addColorStop(0.06, 'rgba(0,0,0,.9)'); cg.addColorStop(0.11, 'rgba(0,0,0,0)');
    cg.addColorStop(0.5, 'rgba(0,0,0,0)'); cg.addColorStop(0.58, 'rgba(0,0,0,.45)'); cg.addColorStop(0.66, 'rgba(0,0,0,0)'); cg.addColorStop(1, 'rgba(0,0,0,0)');
    x.globalCompositeOperation = 'destination-in'; x.fillStyle = cg; x.fillRect(0, 0, sc.width, sc.height);
    ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.globalAlpha = 0.75; ctx.drawImage(sc, 0, 0); ctx.restore();
    // Motes of light drifting up off the halo, swaying a little and fading out.
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    for (var m = 0; m < 34; m++) {
      var life = 3 + 2.5 * hash(m, 2), ph = fract(t / life + hash(m, 1)), id = m * 31 + Math.floor(t / life + hash(m, 1));
      var ma = hash(id, 3) * TAU, mr = R1 * (0.94 + 0.12 * hash(id, 4));
      var mx = cx + Math.cos(ma) * mr + Math.sin(ph * 5 + id) * S1 * 0.006, my = cy + Math.sin(ma) * mr - ph * S1 * (0.05 + 0.05 * hash(id, 6));
      var al = smooth(0, 0.25, ph) * (1 - smooth(0.5, 1, ph)) * (0.55 + 0.45 * Math.sin(t * 4 + m * 1.7));
      var ms = S1 * (0.003 + 0.004 * hash(id, 5));
      var mg = ctx.createRadialGradient(mx, my, 0, mx, my, ms * 2.5);
      mg.addColorStop(0, rgba(pale, al)); mg.addColorStop(0.35, rgba(gold, al * 0.35)); mg.addColorStop(1, rgba(gold, 0));
      ctx.fillStyle = mg; ctx.beginPath(); ctx.arc(mx, my, ms * 2.5, 0, TAU); ctx.fill();
    }
    ctx.restore();
    // Warm light falling on the picture's edge from the halo.
    var Rin = R1 - G.rw;
    ctx.save(); ctx.beginPath(); ctx.arc(cx, cy, Rin, 0, TAU); ctx.clip();
    var ig = ctx.createRadialGradient(cx, cy, Rin * 0.75, cx, cy, Rin);
    ig.addColorStop(0, rgba(gold, 0)); ig.addColorStop(1, rgba(gold, 0.2 + 0.04 * Math.sin(t * 0.9)));
    ctx.globalCompositeOperation = 'lighter'; ctx.fillStyle = ig; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
  }
  EXTRA.push({ id: 'divine', name: 'Divine', blurb: 'A gilded halo from an old altarpiece, tooled by hand, in a slowly turning glory of light.',
    key: [222, 170, 80], luma: 0.3, spread: 0.6,
    spec: { anim: true, roundOnly: true, scale: 0.87, mat: DIVINE_MAT, back: divineBack, front: divineFront } });

  /* ---------- Ink ring: one brush stroke round the token ----------
     A single loaded brush taken once round the circle. It goes down blunt, pressed flat, with no pointed start; it
     runs wet and solid, then dries as it goes, the ink thinning into the split lines of the bristles, and its dry
     tail runs on over the start, so the ring closes on itself where the brush lifted. Unlike the ink on washi, whose
     stroke tapers to a point, this one keeps its width the whole way round. A few drops fly off where it lifted. */
  // Each stroke is drawn whole on a scratch canvas and laid on with the ink's strength, so a stroke has no seams
  // in it and two strokes that overlap come out darker, as wet ink over wet ink does.
  var inkringScratch = null;
  function inkringStroke(g, cx, cy, R0, S0, ink, st) {
    var w = g.canvas.width, h = g.canvas.height;
    if (!inkringScratch) inkringScratch = document.createElement('canvas');
    var T = inkringScratch; if (T.width !== w || T.height !== h) { T.width = w; T.height = h; }
    var x = T.getContext('2d'); x.clearRect(0, 0, w, h); x.lineCap = 'round'; x.lineJoin = 'round';
    var steps = Math.max(40, Math.round(st.sweep * 60)), bristles = st.bristles;
    function width(t) { // pressed in at the start, swelling, then lifting toward the end
      return st.w * (0.9 + 0.15 * Math.exp(-t * 6) - st.taper * t) * (1 + 0.07 * Math.sin(t * 7 + st.seed));
    }
    function rad(t) { return R0 + st.dr + st.drift * t + S0 * 0.002 * Math.sin(t * 4 + st.seed * 3); }
    function pt(t, off) { var a = st.a + st.sweep * t, r = rad(t) + off * width(t); return [cx + Math.cos(a) * r, cy + Math.sin(a) * r]; }
    // The wet body: one solid shape, fading out along the stroke as the brush runs dry.
    var k, top = [], bot = [];
    // The brush goes down pressed flat, so the start is blunt and rounded: each part of the brush's width starts a
    // little later the further it is from the middle (c is half the brush's width, measured along the stroke).
    var c = st.w * 0.5 / (R0 * st.sweep);
    function entry(off) { var q = Math.min(1, Math.abs(off) / 0.5); return c * (1 - Math.sqrt(1 - q * q)); }
    // The wet body stops short of a full turn: the fade below goes by angle, so a body that ran on past the start
    // would light up again over it. Past that point the stroke is only its dry bristles.
    var tMax = Math.min(1, (TAU - 0.12) / st.sweep);
    for (k = 0; k <= steps; k++) { var t = c * 0.6 + k / steps * (tMax - c * 0.6); top.push(pt(t, -0.3)); bot.push(pt(t, 0.3)); }
    x.fillStyle = rgba(ink, 0.75); x.beginPath(); x.moveTo(top[0][0], top[0][1]);
    for (k = 1; k < top.length; k++) x.lineTo(top[k][0], top[k][1]);
    for (k = bot.length - 1; k >= 0; k--) x.lineTo(bot[k][0], bot[k][1]);
    for (k = 1; k < 12; k++) { var o = 0.3 - 0.6 * k / 12, e = pt(entry(o * 1.667) * 0.6, o); x.lineTo(e[0], e[1]); } // the rounded heel
    x.closePath(); x.fill();
    var fade = x.createConicGradient(st.a - 0.05, cx, cy), span = st.sweep / TAU;
    for (k = 0; k <= 20; k++) { var tf = k / 20 * tMax; fade.addColorStop(Math.min(1, (0.05 / TAU) + tf * span), 'rgba(0,0,0,' + (1 - smooth(st.dryAt - 0.12, st.dryAt + 0.1, tf)).toFixed(3) + ')'); }
    fade.addColorStop(Math.min(1, (0.05 / TAU) + tMax * span + 0.001), 'rgba(0,0,0,0)');
    x.globalCompositeOperation = 'destination-in'; x.fillStyle = fade; x.fillRect(0, 0, w, h); x.globalCompositeOperation = 'source-over';
    // The bristles: each leaves its own line; toward the end, and at the stroke's edges first, they skip.
    for (var b = 0; b < bristles; b++) {
      var off = b / (bristles - 1) - 0.5, lw = Math.max(0.6, S0 * 0.0021) * (0.6 + hash(b + st.seed * 97, 3) * 0.9);
      x.strokeStyle = rgba(ink, 0.35 + 0.6 * Math.pow(hash(b + st.seed * 31, 9), 0.6)); x.lineWidth = Math.max(lw, st.w / bristles * 2.2 * (0.7 + 0.6 * hash(b, 5)));
      // Each unbroken run of a bristle is one line, so the ink doesn't double up where segments meet.
      var run = false;
      for (k = 0; k <= steps; k++) {
        var tb = k / steps, q = pt(tb, off + 0.05 * (vnoise(b * 3.1 + st.seed, tb * 9 * st.sweep) - 0.5));   // bristles wander, so the edges are ragged
        var dry = vnoise(b * 1.7 + st.seed * 13, tb * 4.5 * st.sweep) < smooth(st.dryAt - 0.2, 1, tb) * 0.62 + Math.abs(off) * 0.45 * smooth(0.1, 1, tb);
        var on = !dry && Math.abs(off) < 0.5 * (1.05 - 0.25 * tb) && tb >= entry(off) + c * 0.15 * hash(b, 7);
        if (on && !run) { x.beginPath(); x.moveTo(q[0], q[1]); run = true; }
        else if (on) x.lineTo(q[0], q[1]);
        else if (run) { x.stroke(); run = false; }
      }
      if (run) x.stroke();
    }
    // A few drops thrown off where the brush lifted away.
    for (var s = 0; s < st.drops; s++) {
      var ta = st.a + st.sweep * (0.85 + 0.2 * hash(s, st.seed)), tr = rad(1) + (hash(s, st.seed + 1) - 0.4) * S0 * 0.05, ds = S0 * (0.002 + 0.004 * hash(s, st.seed + 2));
      x.beginPath(); x.arc(cx + Math.cos(ta) * tr, cy + Math.sin(ta) * tr, ds, 0, TAU); x.fill();
    }
    g.save(); g.globalAlpha = st.ink; g.drawImage(T, 0, 0); g.restore();
  }
  function inkringLayer(w, h) {
    var key = 'inkring|' + w + '|' + h;
    if (layerCache.has(key)) return layerCache.get(key);
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), S0 = Math.min(w, h), cx = w / 2, cy = h / 2, R0 = S0 * 0.4, ink = [14, 14, 20];
    g.save(); g.lineCap = 'round';
    inkringStroke(g, cx, cy, R0, S0, ink, {
      a: -2.3, sweep: TAU + 0.45, seed: 1, w: S0 * 0.082, dr: 0, drift: -S0 * 0.006, taper: 0.1,
      ink: 0.94, dryAt: 0.8, bristles: 90, drops: 4
    });
    g.restore();
    layerCache.set(key, cv);
    return cv;
  }
  // A faint dark shadow under the ink, kept out of the recolouring, so pale ink still reads on a pale map.
  function inkringShade(w, h) {
    var key = 'inkringShade|' + w + '|' + h;
    if (layerCache.has(key)) return layerCache.get(key);
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), S0 = Math.min(w, h);
    g.filter = 'blur(' + Math.max(1, S0 * 0.01) + 'px)'; g.globalAlpha = 0.22; g.drawImage(inkringLayer(w, h), S0 * 0.004, S0 * 0.008);
    layerCache.set(key, cv);
    return cv;
  }
  EXTRA.push({ id: 'inkring', name: 'Ink ring', blurb: 'One brush stroke taken round the token, wet and solid, drying into split bristles where it closes.',
    key: [14, 14, 20], luma: 1, spec: { roundOnly: true, rim: 0.12, inset: inkInset,
      front: function (ctx, G) {
        if (GRADE) ctx.drawImage(inkringShade(G.bw, G.bh), G.ox, G.oy);
        ctx.drawImage(graded('inkring', inkringLayer(G.bw, G.bh)), G.ox, G.oy);
      } } });
  /* ---------- Molten ring ----------
     A ring of cooling lava seen from above: a crust of dark plates riding on molten rock, drifting round the ring
     at slightly different speeds so the plates slide past each other and jostle. Where neighbours pull apart the
     rock between them shows through, hottest in the middle of the gap and cooling to dull red at the crust's edge;
     where they push together the crack closes to a dim seam. The plates are a Voronoi tiling whose seeds move, so
     every crack opens, narrows and shifts continuously. Each plate has its own tilt, sheen, rough or glassy skin
     and ragged edge, and is lit from the top left and, on the faces toward a crack, by the crack. A fine mesh of
     cooling cracks crazes every plate and glows where the ring runs hot. The field is worked out once per frame on
     a small grid in the ring's own space, then lit and coloured under whatever grade is in force. */
  var LAVA_N = 320, LAVA_R0 = 0.4145, LAVA_PIC = 0.384, LAVA_PIX = 10, LAVA_HN = 512, LAVA_TILE = 256, LAVA_TF = 300, LAVA_TG = 294.9;
  // The flow streaks go round the ring in whole tile widths so they close without a seam.
  var LAVA_FLOW = 3 * 256 / (2 * Math.PI);
  var LAVA_ROWS = [{ n: 30, v: 0.3935, w: 0.03 }, { n: 33, v: 0.4405, w: 0.02 }];
  var lavaF = { q: -1, pk: null, tq: -1, cv: null, img: null };
  var lavaS = null, lavaSkin = null;
  // exp(-x) and the smoothstep curve are needed for every pixel, so they come from a table and a short polynomial.
  var LAVA_EXP = (function () { var a = new Float32Array(258); for (var i = 0; i < 258; i++) a[i] = Math.exp(-i / 32); return a; })();
  function lavaExp(x) { if (x <= 0) return 1; x *= 32; if (x >= 256) return 0; var i = x | 0; return LAVA_EXP[i] + (LAVA_EXP[i + 1] - LAVA_EXP[i]) * (x - i); }
  function lavaSm(x) { return x <= 0 ? 0 : x >= 1 ? 1 : x * x * (3 - 2 * x); }
  function lavaBil(a, o00, o10, o01, o11, c, fx, fy) {
    var p = a[o00 + c];
    return p + (a[o10 + c] - p) * fx + (a[o01 + c] - p) * fy + (p - a[o10 + c] - a[o01 + c] + a[o11 + c]) * fx * fy;
  }

  // A tileable skin for the plates, baked once and read per pixel in each plate's own frame so grain and cracks ride
  // with the plate. Four channels share one lookup: the surface relief (folds along the ring and pits), a ragged-edge
  // wobble, streaks of cooling skin for the molten rock, and the distance to the nearest hairline crack.
  function lavaTN(x, y, px, py, ox, oy) { // value noise on a lattice that repeats every px by py cells
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi, mx = px - 1, my = py - 1;
    var x0 = (xi & mx) + ox, x1 = ((xi + 1) & mx) + ox, y0 = (yi & my) + oy, y1 = ((yi + 1) & my) + oy;
    var a = lat(x0, y0), b = lat(x1, y0), c = lat(x0, y1), d = lat(x1, y1), u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
    return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v;
  }
  // Distance to the nearest edge of a tiling of polygons c texels across, so cracks meet three at a junction.
  function lavaEdge(x, y, c, seed) {
    var m = 256 / c - 1, cx = Math.floor(x / c), cy = Math.floor(y / c), D1 = 1e9, D2 = 1e9, a1x = 0, a1y = 0, a2x = 0, a2y = 0;
    for (var j = -1; j <= 1; j++) for (var i = -1; i <= 1; i++) {
      var gx = cx + i, gy = cy + j, px = (gx + 0.12 + 0.76 * lat((gx & m) + seed, (gy & m) + 9)) * c, py = (gy + 0.12 + 0.76 * lat((gy & m) + 100, (gx & m) + seed + 17)) * c;
      var dx = px - x, dy = py - y, D = dx * dx + dy * dy;
      if (D < D1) { D2 = D1; a2x = a1x; a2y = a1y; D1 = D; a1x = px; a1y = py; } else if (D < D2) { D2 = D; a2x = px; a2y = py; }
    }
    return (D2 - D1) / (2 * Math.hypot(a2x - a1x, a2y - a1y) + 1e-6);
  }
  function lavaMakeSkin() {
    var n = LAVA_TILE, a = new Float32Array(n * n * 4);
    for (var y = 0; y < n; y++) for (var x = 0; x < n; x++) {
      var o = (y * n + x) * 4;
      var wx = x + 22 * (lavaTN(x / 32, y / 32, 8, 8, 13, 29) - 0.5), wy = y + 22 * (lavaTN(x / 32, y / 32, 8, 8, 57, 3) - 0.5);
      var r1 = 1 - Math.abs(2 * lavaTN(wx / 16, wy / 8, 16, 32, 0, 0) - 1), r2 = 1 - Math.abs(2 * lavaTN(wx / 8, wy / 4, 32, 64, 40, 9) - 1);
      var pn = lavaTN(x / 4, y / 4, 64, 64, 11, 70) * 0.7 + lavaTN(x / 2, y / 2, 128, 128, 90, 3) * 0.3;
      a[o] = 0.0009 * (0.62 * r1 + 0.38 * r2) - 0.0014 * smooth(0.62, 0.8, pn);
      a[o + 1] = 0.65 * lavaTN(x / 32, y / 32, 8, 8, 7, 31) + 0.35 * lavaTN(x / 16, y / 16, 16, 16, 61, 5);
      a[o + 2] = 0.6 * lavaTN(x / 16, y / 4, 16, 64, 3, 55) + 0.4 * lavaTN(x / 8, y / 2, 32, 128, 25, 17);
      var bx = x + 9 * (lavaTN(x / 32, y / 32, 8, 8, 31, 61) - 0.5) + 2.5 * (lavaTN(x / 8, y / 8, 32, 32, 7, 90) - 0.5), by = y + 9 * (lavaTN(x / 32, y / 32, 8, 8, 77, 5) - 0.5) + 2.5 * (lavaTN(x / 8, y / 8, 32, 32, 40, 12) - 0.5);
      a[o + 3] = Math.min(lavaEdge(bx, by, 16, 3), 0.55 * lavaEdge(bx + 40, by + 17, 32, 11)) / LAVA_TF;
    }
    return a;
  }

  // The ring's crust pixels, with what never changes between frames: where each sits, how far inside the ragged inner
  // and outer edges, and how far the plates' outlines are bent there. And the plates themselves, each with its own
  // size, tilt, sheen and rhythm.
  function lavaSetup() {
    var N = LAVA_N, list = [];
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, r = Math.hypot(X, Y);
      if (r < 0.352 || r > 0.478) continue;
      var cx = X / r, cy = Y / r;
      var bi = 0.3685 + 0.034 * (qfbm(cx * 2.3 + 31, cy * 2.3 + 7, 3) - 0.5) + 0.006 * (qn(cx * 21 + 3, cy * 21 + 8) - 0.5);
      var bo = 0.4565 + 0.04 * (qfbm(cx * 2.7 + 9, cy * 2.7 + 51, 3) - 0.5) + 0.008 * (qn(cx * 33 + 12, cy * 33 + 4) - 0.5);
      var mb = Math.min(r - bi, bo - r);
      if (mb < -0.002) continue;
      var wx = (qfbm(X * 5 + 3.1, Y * 5 + 8.2, 2) - 0.5) * 0.05 + (qn(X * 11 + 6.4, Y * 11 + 2.9) - 0.5) * 0.024 + (qn(X * 29 + 1.4, Y * 29 + 5.9) - 0.5) * 0.007;
      var wy = (qfbm(X * 5 + 19.7, Y * 5 + 1.3, 2) - 0.5) * 0.05 + (qn(X * 11 + 14.1, Y * 11 + 9.3) - 0.5) * 0.024 + (qn(X * 29 + 8.1, Y * 29 + 2.3) - 0.5) * 0.007;
      list.push(y * N + x, Math.atan2(Y, X), r, X, Y, mb, cx, cy, (wy * cx - wx * cy) / LAVA_R0, wx * cx + wy * cy);
    }
    var S = { px: new Float32Array(list), count: 0 };
    LAVA_ROWS.forEach(function (row) { row.off = S.count; S.count += row.n; });
    var M = S.count;
    ['phi0', 'v0', 'wgt', 'tone', 'gloss', 'tx', 'ty', 'hp', 'nu', 'ph', 'ju', 'jv', 'su', 'sv', 'sn', 'cs', 'pu', 'pv', 'hs'].forEach(function (k) { S[k] = new Float32Array(M); });
    S.rowOf = new Uint8Array(M);
    LAVA_ROWS.forEach(function (row, j) {
      var sp = TAU / row.n;
      for (var k = 0; k < row.n; k++) {
        var m = row.off + k, h = function (c) { return hash(m * 3.17 + c, j * 7.3 + c * 1.9); };
        S.rowOf[m] = j;
        S.phi0[m] = (k + 0.5 + (h(1) - 0.5) * 0.9) * sp - Math.PI; S.v0[m] = row.v + (h(2) - 0.5) * 0.05;
        S.wgt[m] = 0.008 * Math.pow(h(3), 2.5);
        S.tone[m] = 0.5 + 0.75 * h(4); S.gloss[m] = h(5) < 0.5 ? 0.6 + 0.4 * h(6) : 0.3 * h(6);
        S.tx[m] = (h(7) - 0.5) * 0.2; S.ty[m] = (h(8) - 0.5) * 0.2; S.hp[m] = 0.0015 + 0.001 * h(9);
        S.nu[m] = 0.35 + 0.55 * h(10); S.ph[m] = h(11) * TAU; S.pu[m] = h(12) * TAU; S.pv[m] = h(13) * TAU;
      }
    });
    S.hot = new Float32Array(LAVA_HN);
    lavaS = S;
    return S;
  }

  // Seed positions at time t: each plate's slow drift round the ring plus a small orbit that jostles it; and how
  // hot the ring runs at each angle, which wanders slowly.
  function lavaSeeds(t) {
    var S = lavaS, m;
    for (m = 0; m < S.count; m++) {
      var row = LAVA_ROWS[S.rowOf[m]], pu = S.pu[m] + t * (0.23 + 0.2 * S.nu[m]), pv = S.pv[m] + t * (0.19 + 0.2 * S.nu[m]);
      var ju = 0.013 * Math.sin(pu), jv = 0.008 * Math.sin(pv * 1.3 + 1);
      S.ju[m] = ju; S.jv[m] = jv;
      var a = S.phi0[m] + row.w * t + ju / LAVA_R0;
      S.su[m] = a - TAU * Math.floor((a + Math.PI) / TAU);
      S.sv[m] = S.v0[m] + jv;
      var th = S.nu[m] * t + S.ph[m]; S.sn[m] = Math.sin(th); S.cs[m] = Math.cos(th);
      S.hs[m] = qn(S.pu[m] * 1.7 + t * 0.07, S.pv[m] * 1.7 + 3);
    }
    for (m = 0; m < LAVA_HN; m++) {
      var c = Math.cos(m * TAU / LAVA_HN), s = Math.sin(m * TAU / LAVA_HN);
      S.hot[m] = smooth(0.3, 0.7, qn(c * 1.7 + 11 + t * 0.04, s * 1.7 + 4) * 0.62 + qn(c * 3.6 + 2.2, s * 3.6 + 9.1 - t * 0.05) * 0.38);
    }
  }

  // The hot colours as a lookup from temperature, and the crust's colours, under the grade in force.
  var lavaPals = {};
  var LAVA_RAMP = [[0, [10, 1, 1]], [0.1, [70, 6, 3]], [0.24, [150, 18, 6]], [0.42, [226, 58, 10]], [0.58, [250, 108, 18]], [0.74, [255, 168, 52]], [0.88, [255, 222, 128]], [1, [255, 246, 212]]];
  function lavaPal() {
    var k = gradeId();
    if (lavaPals[k]) return lavaPals[k];
    var ramp = LAVA_RAMP.map(function (e) { return [e[0], tc(e[1])]; }), lut = new Float32Array(256 * 3);
    for (var i = 0; i < 256; i++) { var c = rampAt(ramp, i / 255); lut[i * 3] = c[0]; lut[i * 3 + 1] = c[1]; lut[i * 3 + 2] = c[2]; }
    return (lavaPals[k] = { lut: lut, rough: tc([54, 50, 47]), glass: tc([19, 19, 23]), warm: tc([255, 96, 26]), glow: tc([255, 104, 30]), hot: tc([255, 190, 80]), white: tc([255, 240, 200]) });
  }

  function lavaField(t) {
    var F = lavaF, q = Math.round(t * 30);
    if (F.q === q) return F;
    var S = lavaS || lavaSetup(), N = LAVA_N;
    if (!lavaSkin) lavaSkin = lavaMakeSkin();
    if (!F.cv) {
      F.cv = document.createElement('canvas'); F.cv.width = F.cv.height = N; F.img = F.cv.getContext('2d').createImageData(N, N);
      ['H', 'T', 'CM', 'AL', 'TN', 'GS', 'GL', 'NX', 'NY'].forEach(function (k) { F[k] = new Float32Array(N * N); });
    }
    lavaSeeds(t);
    var PX = S.px, np = PX.length, H = F.H, T = F.T, CM = F.CM, AL = F.AL, TN = F.TN, GS = F.GS, GL = F.GL, NXa = F.NX, NYa = F.NY, HOT = S.hot, SK = lavaSkin;
    var SU = S.su, SV = S.sv, SW = S.wgt, JU = S.ju, JV = S.jv, R0 = LAVA_R0, PI = Math.PI, HS = S.hs, TF = LAVA_TF, TM = LAVA_TILE - 1, TW = LAVA_TILE * 4;
    var n0 = LAVA_ROWS[0].n, n1 = LAVA_ROWS[1].n, o1 = LAVA_ROWS[1].off, TG = LAVA_FLOW;
    var dr0 = (LAVA_ROWS[0].w * t) % TAU, dr1 = (LAVA_ROWS[1].w * t) % TAU, is0 = n0 / TAU, is1 = n1 / TAU, hk = LAVA_HN / TAU, flow = t * 5.5;
    for (var k = 0; k < np; k += LAVA_PIX) {
      var i = PX[k], phi = PX[k + 1], r = PX[k + 2], mb = PX[k + 5], ux = PX[k + 6], uy = PX[k + 7];
      var phiW = phi + PX[k + 8], rW = r + PX[k + 9];
      var D1 = 9, D2 = 9, i1 = 0, i2 = 0, j, dk, kk, idx, dp, du, dv, D, a, k0;
      for (j = 0; j < 2; j++) {
        var nj = j ? n1 : n0, off = j ? o1 : 0;
        a = phiW + PI - (j ? dr1 : dr0); if (a < 0) a += TAU; if (a < 0) a += TAU; else if (a >= TAU) a -= TAU;
        k0 = (a * (j ? is1 : is0)) | 0;
        for (dk = -1; dk <= 1; dk++) {
          kk = k0 + dk; if (kk < 0) kk += nj; else if (kk >= nj) kk -= nj;
          idx = off + kk; dp = phiW - SU[idx]; if (dp > PI) dp -= TAU; else if (dp < -PI) dp += TAU;
          du = dp * R0; dv = rW - SV[idx]; D = du * du + dv * dv - SW[idx];
          if (D < D1) { D2 = D1; i2 = i1; D1 = D; i1 = idx; } else if (D < D2) { D2 = D; i2 = idx; }
        }
      }
      var dpa = phiW - SU[i1]; if (dpa > PI) dpa -= TAU; else if (dpa < -PI) dpa += TAU;
      var lu = dpa * R0, lv = rW - SV[i1];
      var abu = SU[i2] - SU[i1]; if (abu > PI) abu -= TAU; else if (abu < -PI) abu += TAU; abu *= R0;
      var abv = SV[i2] - SV[i1], iab = 1 / (Math.sqrt(abu * abu + abv * abv) + 1e-6), e = (D2 - D1) * 0.5 * iab;
      var ex = abu * iab, ey = abv * iab, delta = (JU[i2] - JU[i1]) * ex + (JV[i2] - JV[i1]) * ey;
      // Plate-local tile lookup: ragged edge first, since it decides what else is needed.
      var tx = lu * TF + S.pu[i1] * 40, ty = lv * TF + S.pv[i1] * 40, ix = Math.floor(tx), iy = Math.floor(ty), fx = tx - ix, fy = ty - iy;
      var sx0 = (ix & TM) * 4, sx1 = ((ix + 1) & TM) * 4, sy0 = (iy & TM) * TW, sy1 = ((iy + 1) & TM) * TW;
      var o00 = sy0 + sx0, o10 = sy0 + sx1, o01 = sy1 + sx0, o11 = sy1 + sx1;
      var rg = lavaBil(SK, o00, o10, o01, o11, 1, fx, fy);
      // How open the crack is: pairs pulling apart open it, pairs pushing together close it; hotter stretches of the
      // ring run wider, and the width wanders along the crack so it tapers and pinches out.
      var hot = 0.6 * HOT[(((phi + PI) * hk) | 0) & (LAVA_HN - 1)] + 0.4 * (HS[i1] + HS[i2]);
      var open = delta * 28 + 0.55; open = open < 0 ? 0 : open > 1.2 ? 1.2 : open;
      var w = 0.07 * open * (0.15 + 1.2 * hot) * (0.04 + 0.96 * lavaSm((rg - 0.33) * 4)); if (w > 0.04) w = 0.04;
      var pulse = S.sn[i1] * S.cs[i2] + S.cs[i1] * S.sn[i2];
      var T0 = 0.44 + 0.52 * hot + 0.12 * pulse + 0.06 * (S.tone[i1] + S.tone[i2] - 1.3);
      if (mb < 0.02) T0 *= 0.3 + 0.7 * lavaSm(mb * 50);   // and it glows duller there
      var hw = 0.5 * w + (rg - 0.5) * 0.006; if (hw < 0) hw = 0;
      hw *= 0.4 + 0.6 * lavaSm((mb - 0.001 + (rg - 0.5) * 0.008) * 80);   // the ring's own edge is cooler, with a thinner gap
      var ec = e - hw, gl = S.gloss[i1], hgt = 0, Tm = 0, gln = 0;
      var cm = ec <= -0.003 ? 0 : ec >= 0.003 ? 1 : lavaSm((ec + 0.003) * 166.7);
      var al = lavaSm((mb + (rg - 0.5) * 0.007 + 0.002) * 250);
      if (ec < 0.003) {   // molten rock, cooling to dull red toward the crust's edge
        var zz = ec < 0 ? e / (hw + 1e-5) : 1; zz *= zz;
        // Streaks of cooling skin flow round the ring through the gaps, in the ring's frame, not the plate's.
        var gx = phi * TG + flow, gy = r * LAVA_TG, gi = Math.floor(gx), gj = Math.floor(gy), gfx = gx - gi, gfy = gy - gj;
        var g00 = ((gj & TM) * TW) + (gi & TM) * 4, g10 = ((gj & TM) * TW) + ((gi + 1) & TM) * 4, g01 = (((gj + 1) & TM) * TW) + (gi & TM) * 4, g11 = (((gj + 1) & TM) * TW) + ((gi + 1) & TM) * 4;
        var sk = lavaBil(SK, g00, g10, g01, g11, 2, gfx, gfy);
        var hx = gx * (7 / 3) - flow * 0.6 + 71, hy = gy * 2.3 + 13, hi = Math.floor(hx), hj = Math.floor(hy), hfx = hx - hi, hfy = hy - hj;
        var h00 = ((hj & TM) * TW) + (hi & TM) * 4, h10 = ((hj & TM) * TW) + ((hi + 1) & TM) * 4, h01 = (((hj + 1) & TM) * TW) + (hi & TM) * 4, h11 = (((hj + 1) & TM) * TW) + ((hi + 1) & TM) * 4;
        var sk2 = lavaBil(SK, h00, h10, h01, h11, 2, hfx, hfy);
        Tm = ec < 0 ? T0 * (1 - 0.72 * zz) * (0.66 + 0.5 * sk + 0.3 * sk2) * (1 - 0.85 * (0.25 + 0.75 * zz) * lavaSm((sk * 0.65 + sk2 * 0.35 - 0.48) * 5)) : T0 * (0.04 + 0.24 * Math.min(1, hw * 250)) * lavaExp(ec * 208);
      } else if (ec < 0.03) Tm = T0 * (0.04 + 0.24 * Math.min(1, hw * 250)) * lavaExp(ec * 208);
      if (ec > -0.003) {  // the plate: bevelled at its edge, tilted, folded and pitted on top
        var rel = SK[o00] + (SK[o10] - SK[o00]) * fx + (SK[o01] - SK[o00]) * fy + (SK[o00] - SK[o10] - SK[o01] + SK[o11]) * fx * fy;
        var cz = lavaBil(SK, o00, o10, o01, o11, 3, fx, fy), line = 1 - lavaSm(cz * 250);
        var bev = ec <= 0 ? 0 : ec >= 0.0026 ? 1 : lavaSm(ec * 385), fr = ec < 0.04 ? lavaExp(ec * 143) : 0;
        hgt = bev * (S.hp[i1] + S.tx[i1] * lu + S.ty[i1] * lv + rel * (1 - 0.6 * gl) + 0.0014 * fr - 0.0012 * line);
        var mbe = mb + (rg - 0.5) * 0.006; hgt *= mbe <= 0 ? 0 : mbe >= 0.004 ? 1 : lavaSm(mbe * 250);
        // Hairline cracks: a plate's mesh glows where the ring runs hot and flickers on and off with the plate.
        if (line > 0.01 && ec > 0.0015) Tm += T0 * 0.75 * line * (0.2 + 0.8 * hot) * lavaSm((0.45 * rg + 0.55 * (0.5 + 0.5 * S.sn[i1]) - 0.28) * 3.333) * (ec >= 0.008 ? 1 : lavaSm((ec - 0.0015) * 153.8));
        if (ec < 0.09) gln = T0 * (0.6 * lavaExp(ec * 200) + 0.4 * lavaExp(ec * 50)) * (0.1 + 0.9 * Math.min(1, hw * 200));
      }
      H[i] = hgt; T[i] = Tm; CM[i] = cm; AL[i] = al; TN[i] = S.tone[i1]; GS[i] = gl; GL[i] = gln;
      NXa[i] = -ex * uy + ey * ux; NYa[i] = ex * ux + ey * uy;   // toward the crack, in screen space
    }
    F.q = q; F.pk = null;
    return F;
  }

  function lavaTexture(t) {
    var F = lavaField(t), pk = gradeId();
    if (F.pk === pk && F.tq === F.q) return F.cv;
    var S = lavaS, N = LAVA_N, pal = lavaPal(), d = F.img.data, PX = S.px, np = PX.length;
    var H = F.H, T = F.T, CM = F.CM, AL = F.AL, TN = F.TN, GS = F.GS, GL = F.GL, NXa = F.NX, NYa = F.NY, lut = pal.lut;
    var rg = pal.rough, gs = pal.glass, wm = pal.warm, L = LIGHT, HF = HALF, hs = N * 0.5;
    for (var k = 0; k < np; k += LAVA_PIX) {
      var i = PX[k], o = i * 4, cm = CM[i], ti = T[i], R, G, B;
      ti = (ti < 0 ? 0 : ti > 1 ? 255 : ti * 255) | 0;
      var ti3 = ti * 3; d[o + 3] = 255 * AL[i];
      if (cm < 0.003) { d[o] = lut[ti3]; d[o + 1] = lut[ti3 + 1]; d[o + 2] = lut[ti3 + 2]; continue; }
      var gx = (H[i + 1] - H[i - 1]) * hs, gy = (H[i + N] - H[i - N]) * hs, inl = 1 / Math.sqrt(gx * gx + gy * gy + 1);
      var nx = -gx * inl, ny = -gy * inl, nz = inl, gl = GS[i], tn = TN[i];
      var diff = nx * L[0] + ny * L[1] + nz * L[2]; if (diff < 0) diff = 0;
      var sh = nx * HF[0] + ny * HF[1] + nz * HF[2]; if (sh < 0) sh = 0; sh *= sh; sh *= sh; sh *= sh;      // ^8
      var sh2 = sh * sh; sh2 *= sh2; sh2 *= sh2;                                                          // ^64
      var bs = nx * -0.3 + ny * -0.4 + nz * 0.86; if (bs < 0) bs = 0; bs *= bs; bs *= bs; bs *= bs;      // broad sheen
      var sp = 255 * (sh * (1 - gl) * 0.05 + sh2 * gl * 0.4 + bs * gl * 0.2), lit = (0.15 + 1.0 * diff) * tn;
      R = (rg[0] + (gs[0] - rg[0]) * gl) * lit + sp; G = (rg[1] + (gs[1] - rg[1]) * gl) * lit + sp; B = (rg[2] + (gs[2] - rg[2]) * gl) * lit + sp;
      var gg = GL[i];
      if (gg > 0.004) {   // light from the nearest crack onto the faces that look toward it
        var hd = nx * NXa[i] * 0.93 + ny * NYa[i] * 0.93 + nz * 0.36; if (hd < 0) hd = 0;
        var hl = gg * (0.1 + 0.9 * hd) * 0.5; R += wm[0] * hl; G += wm[1] * hl; B += wm[2] * hl;
      }
      d[o] = lut[ti3] + cm * R; d[o + 1] = lut[ti3 + 1] + cm * G; d[o + 2] = lut[ti3 + 2] + cm * B;
    }
    F.cv.getContext('2d').putImageData(F.img, 0, 0); F.pk = pk; F.tq = F.q;
    return F.cv;
  }

  // The glow the ring throws: the texture shrunk to a few dozen pixels (which blurs it for free), added back over
  // the crust, the map and the picture's edge, so the light follows the hot stretches of the ring.
  var lavaHz = {};
  function lavaHaze(tex, n) {
    var k = n + '#' + gradeId(), h = lavaHz[k];
    if (!h) { h = lavaHz[k] = { cv: document.createElement('canvas'), q: -2 }; h.cv.width = h.cv.height = n; }
    if (h.q !== lavaF.q) { var g = h.cv.getContext('2d'); g.clearRect(0, 0, n, n); g.imageSmoothingQuality = 'high'; g.drawImage(tex, 0, 0, n, n); h.q = lavaF.q; }
    return h.cv;
  }

  // Gas bubbles swell on the molten rock, burst, and throw a few droplets that cool as they fly. Each slot reuses
  // itself: when its cycle begins it picks a spot that is molten right then, so nothing blooms on bare crust.
  var lavaSpr = {}, lavaSlots = [], LAVA_SLOTS = 11;
  function lavaSprites() {
    var k = gradeId(), c = lavaSpr[k];
    if (c) return c;
    var pal = lavaPal(), glow = document.createElement('canvas'), skin = document.createElement('canvas');
    glow.width = glow.height = 32; skin.width = skin.height = 48;
    var g = glow.getContext('2d'), gr = g.createRadialGradient(16, 16, 0, 16, 16, 16), h = pal.hot, w = pal.white, o = pal.glow;
    gr.addColorStop(0, rgba(w, 1)); gr.addColorStop(0.18, rgba(h, 0.9)); gr.addColorStop(0.5, rgba(o, 0.25)); gr.addColorStop(1, rgba(o, 0));
    g.fillStyle = gr; g.fillRect(0, 0, 32, 32);
    // A bubble: a swollen blister of thin skin that lets the heat through, brightest at its crown, with a bright
    // catch-light and a faint cool edge where the skin is thickest.
    var r = skin.getContext('2d'), sg = r.createRadialGradient(24, 24, 0, 24, 24, 23);
    sg.addColorStop(0, rgba(h, 0.5)); sg.addColorStop(0.55, rgba(o, 0.42)); sg.addColorStop(0.85, rgba(o, 0.14)); sg.addColorStop(1, rgba(o, 0));
    r.fillStyle = sg; r.fillRect(0, 0, 48, 48);
    var hg = r.createRadialGradient(18, 17, 0, 18, 17, 8);
    hg.addColorStop(0, rgba(w, 0.95)); hg.addColorStop(0.5, rgba(h, 0.4)); hg.addColorStop(1, rgba(h, 0));
    r.fillStyle = hg; r.fillRect(0, 0, 48, 48);
    return (lavaSpr[k] = { glow: glow, skin: skin });
  }
  function lavaSparks(ctx, G, t) {
    var F = lavaF, S = lavaS, PX = S.px, np = PX.length / LAVA_PIX, W = G.outW, cx = W / 2, cy = G.outH / 2, spr = lavaSprites(), pal = lavaPal();
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    for (var m = 0; m < LAVA_SLOTS; m++) {
      var life = 1.2 + 1.1 * hash(m, 41), cyc = t / life + hash(m, 42), ci = Math.floor(cyc), p = cyc - ci, st = lavaSlots[m];
      if (!st || st.ci !== ci) {
        st = lavaSlots[m] = { ci: ci, ok: false, x: 0, y: 0, ts: t };
        for (var c = 0; c < 10; c++) {
          var k = Math.floor(hash(m * 7 + c, ci * 3.1 + 5) * np) * LAVA_PIX, i = PX[k];
          if (F.CM[i] < 0.15 && F.T[i] > 0.6) { st.ok = true; st.x = PX[k + 3]; st.y = PX[k + 4]; break; }
        }
      }
      if (!st.ok) continue;
      var an = 0.028 * (t - st.ts), ca = Math.cos(an), sa = Math.sin(an), bx = (st.x * ca - st.y * sa) * W + cx, by = (st.x * sa + st.y * ca) * W + cy;
      var R = W * (0.0042 + 0.0042 * hash(m, 43)), id = m * 17 + ci * 5;
      if (p < 0.6) {                                   // swelling: the blister rises and thins
        var g = p / 0.6, sz = R * (0.3 + 0.7 * g * (2 - g));
        ctx.globalAlpha = Math.min(1, g * 3) * 0.9; ctx.drawImage(spr.skin, bx - sz, by - sz, sz * 2, sz * 2);
      } else {                                         // burst: a flash, and drops fly
        var q = (p - 0.6) / 0.4;
        if (q < 0.25) { var fz = R * 2.4 * (1 - q * 3); ctx.globalAlpha = (1 - q * 4) * 0.9; ctx.drawImage(spr.glow, bx - fz, by - fz, fz * 2, fz * 2); }
        for (var d = 0; d < 4; d++) {
          var da = hash(id, d + 50) * TAU, dv = W * (0.012 + 0.03 * hash(id, d + 60)), e = 1 - Math.pow(1 - q, 2.2);
          var dx = bx + Math.cos(da) * dv * e, dy = by + Math.sin(da) * dv * e, ds = W * (0.0014 + 0.0012 * hash(id, d + 70));
          var cool = Math.min(1, q * 1.3), ci3 = Math.floor((0.95 - 0.7 * cool) * 255) * 3;
          ctx.globalAlpha = (1 - q * q) * 0.95; ctx.drawImage(spr.glow, dx - ds * 3, dy - ds * 3, ds * 6, ds * 6);
          ctx.globalAlpha = (1 - q) * 0.9; ctx.fillStyle = 'rgb(' + (pal.lut[ci3] | 0) + ',' + (pal.lut[ci3 + 1] | 0) + ',' + (pal.lut[ci3 + 2] | 0) + ')';
          ctx.beginPath(); ctx.arc(dx, dy, Math.max(0.5, ds), 0, TAU); ctx.fill();
        }
      }
    }
    ctx.restore();
  }

  function lavaFront(ctx, G, t) {
    var tex = lavaTexture(t), W = G.outW, H = G.outH, br = 0.94 + 0.06 * (qn(t * 2.3, 1.5) * 2 - 1);
    ctx.save(); ctx.imageSmoothingQuality = 'high';
    // The ring's light on the map and the picture's edge, then the crust itself, then the glow that bleeds over it.
    ctx.globalCompositeOperation = 'lighter';
    var cx = W / 2, cy = H / 2, lg = ctx.createRadialGradient(cx, cy, W * 0.3, cx, cy, W * 0.5), gc = lavaPal().glow;
    lg.addColorStop(0, rgba(gc, 0)); lg.addColorStop(0.5, rgba(gc, 0.12 * br)); lg.addColorStop(1, rgba(gc, 0));
    ctx.fillStyle = lg; ctx.fillRect(0, 0, W, H);
    ctx.globalAlpha = br; ctx.drawImage(lavaHaze(tex, 48), 0, 0, W, H);
    ctx.globalAlpha = 0.7 * br; ctx.drawImage(lavaHaze(tex, 16), 0, 0, W, H);
    ctx.globalCompositeOperation = 'source-over'; ctx.globalAlpha = 1;
    ctx.drawImage(tex, 0, 0, W, H);
    ctx.globalCompositeOperation = 'lighter'; ctx.globalAlpha = 0.13 * br; ctx.drawImage(lavaHaze(tex, 64), 0, 0, W, H);
    ctx.restore();
    lavaSparks(ctx, G, t);
  }

  EXTRA.push({ id: 'lava', name: 'Molten lava', blurb: 'A ring of cooling lava: dark crust plates drifting and jostling, glowing cracks opening and closing between them.',
    key: [242, 112, 22], luma: 0, spread: 0.2,
    spec: { anim: true, roundOnly: true, inset: function (G) { return G.outW * (0.5 - LAVA_PIC); }, front: lavaFront } });

  /* ---------- Magic: arcane, prismatic and astral light ---------- */
  function magicInset(G) { return G.bs * 0.115; }
  // A soft dark ring under light effects, so the light still reads on a pale map.
  function magicBacking(ctx, G, a, col) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, g = ctx.createRadialGradient(cx, cy, S0 * 0.36, cx, cy, S0 * 0.52);
    g.addColorStop(0, rgba(col, 0)); g.addColorStop(0.35, rgba(col, a)); g.addColorStop(0.7, rgba(col, a * 0.6)); g.addColorStop(1, rgba(col, 0));
    ctx.fillStyle = g; ctx.fillRect(0, 0, G.outW, G.outH);
  }

  // Prismatic: three ribbons of rainbow light twisting round each other, each a band that turns as it goes, so it
  // narrows to a bright edge and widens to a broad face; white-hot where it faces you, a soft glow round it, and
  // glints where the light catches. With a colour chosen, the ribbons keep only a narrow spread of that hue.
  // The light is worked out once per frame in a small texture and added over the token.
  var PRISM_N = 300, prismTex = { cv: null, img: null, q: -1, k: '' }, PRISM_PX = null;
  function hueRGB(h, out) { // a fast, smooth hue wheel, 0..1 per channel
    h = ((h % 360) + 360) % 360 / 60;
    out[0] = clamp(Math.abs(h - 3) - 1, 0, 1); out[1] = clamp(2 - Math.abs(h - 2), 0, 1); out[2] = clamp(2 - Math.abs(h - 4), 0, 1);
    return out;
  }
  function prismField(t, mono, satk) {
    var q = Math.round(t * 30), k = mono + '|' + satk, P = prismTex, N = PRISM_N;
    if (P.q === q && P.k === k) return P.cv;
    if (!P.cv) { P.cv = document.createElement('canvas'); P.cv.width = P.cv.height = N; P.img = P.cv.getContext('2d').createImageData(N, N); }
    if (!PRISM_PX) {
      var list = [];
      for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) { var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, r = Math.hypot(X, Y); if (r > 0.35 && r < 0.5) list.push(y * N + x, Math.atan2(Y, X), r); }
      PRISM_PX = new Float32Array(list);
    }
    var d = P.img.data, c = [0, 0, 0], Rm = 0.432; d.fill(0);
    for (var i = 0; i < PRISM_PX.length; i += 3) {
      var o = PRISM_PX[i] * 4, a = PRISM_PX[i + 1], r = PRISM_PX[i + 2], R = 0, Gc = 0, B = 0;
      for (var kk = 0; kk < 3; kk++) {
        var ph = kk * TAU / 3, rc = Rm + 0.021 * Math.sin(a * 3 + ph + t * (0.5 + 0.15 * kk));
        var face = Math.abs(Math.cos(a * 2 + ph * 1.7 - t * 1.1)), w = 0.004 + 0.016 * face, dd = (r - rc) / w;
        var core = Math.exp(-dd * dd * 1.6) * (0.3 + 0.7 * face * face), glow = Math.exp(-Math.pow((r - rc) / 0.022, 2)) * 0.16;
        if (core + glow < 0.004) continue;
        var hue = mono == null ? a / TAU * 720 + t * 45 + kk * 70 : mono + 22 * Math.sin(a * 2 + t + kk);
        hueRGB(hue, c);
        var wh = core * face * face * 0.8, sat = satk * (1 - wh);
        for (var ch = 0; ch < 3; ch++) c[ch] = c[ch] * sat + (1 - sat);
        var e = core + glow;
        R += c[0] * e; Gc += c[1] * e; B += c[2] * e;
      }
      // Stored unpremultiplied with alpha as the brightness, so adding it over a see-through canvas adds only light.
      var mx = Math.min(1, Math.max(R, Gc, B));
      if (mx < 0.004) continue;
      d[o] = Math.min(255, R / mx * 255); d[o + 1] = Math.min(255, Gc / mx * 255); d[o + 2] = Math.min(255, B / mx * 255); d[o + 3] = mx * 255;
    }
    P.cv.getContext('2d').putImageData(P.img, 0, 0); P.q = q; P.k = k;
    return P.cv;
  }
  function prismaticFront(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, Rm = S0 * 0.432, mono = GRADE ? hueOf(GRADE.target) : null;
    var satk = GRADE ? satOf(GRADE.target) : 1, sat = Math.round(95 * satk);
    ctx.save();
    magicBacking(ctx, G, 0.28, [12, 10, 24]);
    ctx.globalCompositeOperation = 'lighter'; ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(prismField(t, mono, satk), 0, 0, G.outW, G.outH);
    // Glints: four-pointed stars that flare where a ribbon turns full face, and motes drifting between them.
    var glints = [];
    for (var kk = 0; kk < 3; kk++) {
      var ph = kk * TAU / 3, best = (ph * 1.7 - t * 1.1) / 2;                 // where cos(2a + ...) peaks
      for (var j = 0; j < 2; j++) {
        var ga = -best + j * Math.PI, gr0 = Rm + S0 * 0.021 * Math.sin(ga * 3 + ph + t * (0.5 + 0.15 * kk));
        glints.push([cx + Math.cos(ga) * gr0, cy + Math.sin(ga) * gr0, 0.8]);
      }
    }
    for (var m = 0; m < 26; m++) {
      var ma = hash(m, 1) * TAU + t * (0.08 + 0.12 * hash(m, 2)) * (m % 2 ? 1 : -1), mr = Rm + S0 * (hash(m, 3) - 0.5) * 0.09;
      var tw2 = Math.pow(0.5 + 0.5 * Math.sin(t * (1.2 + hash(m, 4) * 2.5) + m * 2.3), 6);
      if (tw2 > 0.05) glints.push([cx + Math.cos(ma) * mr, cy + Math.sin(ma) * mr, tw2]);
    }
    glints.forEach(function (gp, gi) {
      var L = S0 * (0.01 + 0.016 * gp[2]), hue = mono != null ? mono : (gi * 47 + t * 60) % 360;
      var gr = ctx.createRadialGradient(gp[0], gp[1], 0, gp[0], gp[1], L * 0.6);
      gr.addColorStop(0, 'rgba(255,255,255,' + (0.7 * gp[2]).toFixed(3) + ')'); gr.addColorStop(1, 'hsla(' + Math.round(hue) + ',' + sat + '%,70%,0)');
      ctx.fillStyle = gr; ctx.beginPath(); ctx.arc(gp[0], gp[1], L * 0.6, 0, TAU); ctx.fill();
      ctx.strokeStyle = 'rgba(255,255,255,' + (0.5 * gp[2]).toFixed(3) + ')'; ctx.lineWidth = Math.max(0.5, S0 * 0.0014);
      ctx.beginPath(); ctx.moveTo(gp[0] - L, gp[1]); ctx.lineTo(gp[0] + L, gp[1]); ctx.moveTo(gp[0], gp[1] - L * 0.7); ctx.lineTo(gp[0], gp[1] + L * 0.7); ctx.stroke();
    });
    ctx.restore();
    // The picture's edge picks up the light.
    ctx.save(); ctx.beginPath(); ctx.arc(cx, cy, S0 * 0.385, 0, TAU); ctx.clip();
    var ig = ctx.createRadialGradient(cx, cy, S0 * 0.3, cx, cy, S0 * 0.385);
    ig.addColorStop(0, 'rgba(255,255,255,0)'); ig.addColorStop(1, mono != null ? 'hsla(' + Math.round(mono) + ',' + sat + '%,70%,0.2)' : 'hsla(' + Math.round(t * 40 % 360) + ',80%,75%,0.16)');
    ctx.globalCompositeOperation = 'lighter'; ctx.fillStyle = ig; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
  }

  // Astral: a ring of night sky. A band of deep space with nebula clouds drifting in it, stars that turn slowly
  // with it, a few bright ones twinkling with cross-shaped glints, faint lines joining a constellation, and now and
  // then a shooting star. The sky is drawn once (per colour) and turned, so it costs almost nothing to animate.
  var ASTRAL_N = 720, astralSky = {};
  function astralLayer() {
    var k = gradeId();
    if (astralSky[k]) return astralSky[k];
    var N = ASTRAL_N, c = document.createElement('canvas'); c.width = c.height = N;
    var g = c.getContext('2d'), im = g.createImageData(N, N), d = im.data;
    var deep = tc([10, 12, 34]), neb1 = tc([110, 60, 170]), neb2 = tc([40, 110, 190]), neb3 = tc([200, 90, 150]);
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, r = Math.hypot(X, Y), o = (y * N + x) * 4;
      var band = smooth(0.378, 0.392, r) * (1 - smooth(0.478, 0.49, r)), spill = smooth(0.46, 0.49, r) * (1 - smooth(0.49, 0.5, r));
      if (band <= 0 && spill <= 0) continue;
      var w = fbm(X * 6 + 3, Y * 6, 3), n1 = fbm(X * 7 + w * 1.5, Y * 7 - w, 5), n2 = fbm(X * 12 + 9, Y * 12 + w * 2, 4);
      var dust = smooth(0.45, 0.8, n1), glow2 = smooth(0.5, 0.85, n2), lane = smooth(0.55, 0.75, fbm(X * 16 + 4, Y * 16, 3));
      var col = deep.slice();
      for (var q = 0; q < 3; q++) col[q] += neb1[q] * dust * 0.55 + neb2[q] * glow2 * 0.4 * (1 - dust) + neb3[q] * Math.pow(dust * glow2, 2) * 0.6;
      for (q = 0; q < 3; q++) col[q] *= 1 - lane * 0.55;                                     // dark dust lanes across the glow
      d[o] = col[0]; d[o + 1] = col[1]; d[o + 2] = col[2]; d[o + 3] = 255 * Math.max(band * (0.86 + 0.14 * dust), spill * dust * 0.55);                     // nebula spills a little past the edge
    }
    g.putImageData(im, 0, 0);
    // Stars: many faint, fewer bright, warm and cool, thickest where the nebula is.
    for (var s = 0; s < 900; s++) {
      var sa = hash(s, 11) * TAU, sr = N * (0.39 + 0.088 * hash(s, 12)), b = Math.pow(hash(s, 13), 3);
      var sx = N / 2 + Math.cos(sa) * sr, sy = N / 2 + Math.sin(sa) * sr, tint = hash(s, 14);
      var sc = tint < 0.3 ? [255, 214, 180] : tint > 0.75 ? [190, 210, 255] : [255, 250, 240];
      g.fillStyle = rgba(sc, 0.35 + 0.65 * b); g.beginPath(); g.arc(sx, sy, 0.45 + b * 1.4, 0, TAU); g.fill();
    }
    astralSky[k] = c; return c;
  }
  var ASTRAL_STARS = [[0.3, 0.44], [0.36, 0.425], [0.41, 0.455], [0.47, 0.43], [0.52, 0.45], [1.9, 0.43], [1.97, 0.455], [2.05, 0.44], [3.6, 0.42], [3.7, 0.45], [3.78, 0.43], [3.85, 0.445], [5.0, 0.44], [5.1, 0.425]];
  var ASTRAL_LINES = [[0, 1], [1, 2], [2, 3], [3, 4], [5, 6], [6, 7], [8, 9], [9, 10], [10, 11], [12, 13]];
  function astralFront(ctx, G, t) {
    var S0 = G.bs, cx = G.outW / 2, cy = G.outH / 2, rot = t * 0.025, starCol = tc([200, 220, 255]);
    ctx.save();
    magicBacking(ctx, G, 0.3, [4, 4, 12]);
    ctx.translate(cx, cy); ctx.rotate(rot); ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(astralLayer(), -G.outW / 2, -G.outH / 2, G.outW, G.outH);
    // The constellation: faint lines, and its stars twinkling.
    ctx.globalCompositeOperation = 'lighter'; ctx.strokeStyle = rgba(starCol, 0.22); ctx.lineWidth = Math.max(0.5, S0 * 0.0014);
    var P = ASTRAL_STARS.map(function (p) { return [Math.cos(p[0]) * p[1] * S0, Math.sin(p[0]) * p[1] * S0]; });
    ctx.beginPath(); ASTRAL_LINES.forEach(function (l) { ctx.moveTo(P[l[0]][0], P[l[0]][1]); ctx.lineTo(P[l[1]][0], P[l[1]][1]); }); ctx.stroke();
    P.forEach(function (p, i) {
      var tw = 0.55 + 0.45 * Math.sin(t * (1.3 + hash(i, 3) * 2) + i * 2.1), L = S0 * (0.008 + 0.01 * tw);
      var gr = ctx.createRadialGradient(p[0], p[1], 0, p[0], p[1], L); gr.addColorStop(0, 'rgba(255,255,255,' + (0.9 * tw).toFixed(3) + ')'); gr.addColorStop(1, rgba(starCol, 0));
      ctx.fillStyle = gr; ctx.beginPath(); ctx.arc(p[0], p[1], L, 0, TAU); ctx.fill();
      ctx.strokeStyle = 'rgba(255,255,255,' + (0.45 * tw).toFixed(3) + ')';
      ctx.beginPath(); ctx.moveTo(p[0] - L * 1.6, p[1]); ctx.lineTo(p[0] + L * 1.6, p[1]); ctx.moveTo(p[0], p[1] - L * 1.6); ctx.lineTo(p[0], p[1] + L * 1.6); ctx.stroke();
    });
    ctx.restore();
    // A shooting star now and then, streaking round a little way and burning out.
    var slot = Math.floor(t / 3.2), ph = fract(t / 3.2) / 0.22;
    if (ph < 1 && hash(slot, 5) < 0.75) {
      var a0 = hash(slot, 6) * TAU, dir = hash(slot, 7) < 0.5 ? 1 : -1, rr = S0 * (0.4 + 0.07 * hash(slot, 8)), len = 0.5;
      var head = a0 + dir * len * ph, tail = a0 + dir * len * Math.max(0, ph - 0.35), al = Math.sin(Math.PI * ph);
      var hx = cx + Math.cos(head) * rr, hy = cy + Math.sin(head) * rr, tx = cx + Math.cos(tail) * (rr + S0 * 0.01), ty = cy + Math.sin(tail) * (rr + S0 * 0.01);
      ctx.save(); ctx.globalCompositeOperation = 'lighter'; ctx.lineCap = 'round';
      var lg = ctx.createLinearGradient(tx, ty, hx, hy); lg.addColorStop(0, rgba(starCol, 0)); lg.addColorStop(1, 'rgba(255,255,255,' + al.toFixed(3) + ')');
      ctx.strokeStyle = lg; ctx.lineWidth = Math.max(0.8, S0 * 0.003); ctx.beginPath(); ctx.moveTo(tx, ty); ctx.lineTo(hx, hy); ctx.stroke();
      ctx.restore();
    }
    // Starlight on the picture's edge, and a fine bright edge where the sky meets it.
    ctx.save(); ctx.beginPath(); ctx.arc(cx, cy, S0 * 0.385, 0, TAU); ctx.clip();
    var ig = ctx.createRadialGradient(cx, cy, S0 * 0.3, cx, cy, S0 * 0.385);
    ig.addColorStop(0, rgba(starCol, 0)); ig.addColorStop(1, rgba(starCol, 0.16));
    ctx.globalCompositeOperation = 'lighter'; ctx.fillStyle = ig; ctx.fillRect(0, 0, G.outW, G.outH); ctx.restore();
  }

  var magicSpec = function (front) { return { anim: true, roundOnly: true, rim: 0.12, inset: magicInset, front: front }; };
  EXTRA.push({ id: 'arcane', name: 'Arcane', blurb: 'Runes in moving light, lightning round the band and three tiny crystals in orbit.',
    key: [110, 170, 255], luma: 0, spread: 0.4, spec: magicSpec(arcaneFront('blue')) });
  EXTRA.push({ id: 'prismatic', name: 'Prismatic', blurb: 'Three ribbons of rainbow light twisting round each other, glinting as they turn.',
    key: [255, 255, 255], luma: 0, spec: magicSpec(prismaticFront) });
  EXTRA.push({ id: 'astral', name: 'Astral', blurb: 'A ring of night sky: nebulae, slowly turning stars, a constellation and the odd shooting star.',
    key: [110, 90, 220], luma: 0, spread: 0.8, spec: magicSpec(astralFront) });
  /* ---------- Rope ----------
     A thick three-strand hemp rope coiled once round, its two ends bound together with a whipping of thin dark twine.
     The rope is solved as geometry: three round strands wind round its axis (the classic lay), so the height at a point
     is the top of the highest strand there. Each strand is itself laid from twisted yarns, which are laid from fibres,
     and loose fibres stand off the surface. Units below are rope radii. */
  var ROPE_W = 0.108;               // the width of the ring, which is the rope's diameter, as a fraction of the token size
  var ROPE_R = ROPE_W / 2;
  var ROPE_RS = 0.5, ROPE_DS = 0.5; // strand radius, and how far a strand's centre lies from the rope's axis
  var ROPE_LAY = 5.7;               // length of one full turn of the lay, near enough: the real figure is fitted to the ring
  var ROPE_CORNER = 0.07;           // radius of the rope's centre line round a square token's corner
  var ROPE_YARN = 0.62;             // length of one yarn twist along a strand
  var ROPE_KAPPA = 3.2;             // how steeply the yarns lie across a strand
  var WHIP_HALF = 1.05, WHIP_PITCH = 0.12;
  var ropePaths = {};
  var ropeHW = 0.5, ropeHH = 0.5;   // half the frame's width and height in token sizes, learned when the shader starts a pass
  var RP = { s: 0, y: 0 };

  // The rope's path for one shape: round, or a rectangle with rounded corners. s runs clockwise from the left-hand middle, where the whipping is.
  function ropePathFor(isRound) {
    var key = isRound ? 'r' : ropeHW.toFixed(5) + ',' + ropeHH.toFixed(5);
    if (ropePaths[key]) return ropePaths[key];
    var P = { round: isRound, Rc: 0.5 - ROPE_R, ax: ropeHW - ROPE_R, ay: ropeHH - ROPE_R, rc: ROPE_CORNER, cx0: 0, cy0: 0, Q: 0, len: 0, L: 0 };
    if (isRound) P.len = TAU * P.Rc / ROPE_R;
    else { P.cx0 = P.ax - P.rc; P.cy0 = P.ay - P.rc; P.Q = (P.cx0 + P.cy0 + Math.PI * P.rc / 2) / ROPE_R; P.len = 4 * P.Q; }
    P.L = P.len / Math.round(P.len / ROPE_LAY);                        // a whole number of lays round the ring, so the lay has no seam
    return (ropePaths[key] = P);
  }
  // Where a point is on the rope: s along it and y across it (-1 at the outer edge, 1 at the inner), both in rope radii.
  function ropePos(X, Y, P) {
    if (P.round) {
      var th = Math.atan2(Y, X) - Math.PI; if (th < 0) th += TAU;
      RP.s = P.Rc * th / ROPE_R; RP.y = (P.Rc - Math.hypot(X, Y)) / ROPE_R;
      return RP;
    }
    var qx = Math.abs(X), qy = Math.abs(Y), cx0 = P.cx0, cy0 = P.cy0, rc = P.rc, t, sd, dx = qx - cx0, dy = qy - cy0;
    if (dx > 0 && dy > 0) { sd = Math.hypot(dx, dy) - rc; t = cy0 + rc * Math.atan2(dy, dx); }
    else if (dx >= dy) { sd = dx - rc; t = Math.min(qy, cy0); }
    else { sd = dy - rc; t = cy0 + rc * Math.PI / 2 + (cx0 - Math.min(qx, cx0)); }
    var Q = P.Q * ROPE_R;
    RP.s = (X < 0 ? (Y < 0 ? t : 4 * Q - t) : (Y < 0 ? 2 * Q - t : 2 * Q + t)) / ROPE_R;
    RP.y = -sd / ROPE_R;
    return RP;
  }
  // The inverse, for placing loose fibres: the point at s along the rope and y across it, with the rope's direction and its outward normal there.
  var RPT = { X: 0, Y: 0, tx: 0, ty: 0, nx: 0, ny: 0 };
  function ropePoint(P, s, y) {
    var px, py, tx, ty, nx, ny;
    if (P.round) {
      var th = s * ROPE_R / P.Rc + Math.PI, rho = P.Rc - y * ROPE_R;
      nx = Math.cos(th); ny = Math.sin(th); tx = -ny; ty = nx; px = rho * nx; py = rho * ny;
    } else {
      var Q = P.Q * ROPE_R, sq = ((s * ROPE_R) % (4 * Q) + 4 * Q) % (4 * Q), quad = Math.floor(sq / Q), t, cx0 = P.cx0, cy0 = P.cy0, rc = P.rc, qx, qy, cx, cy, ux, uy;
      t = quad === 0 ? sq : quad === 1 ? 2 * Q - sq : quad === 2 ? sq - 2 * Q : 4 * Q - sq;
      if (t < cy0) { qx = P.ax; qy = t; cx = 1; cy = 0; ux = 0; uy = 1; }
      else if (t < cy0 + Math.PI * rc / 2) { var ph = (t - cy0) / rc; cx = Math.cos(ph); cy = Math.sin(ph); qx = cx0 + rc * cx; qy = cy0 + rc * cy; ux = -cy; uy = cx; }
      else { qx = cx0 - (t - cy0 - Math.PI * rc / 2); qy = P.ay; cx = 0; cy = 1; ux = -1; uy = 0; }
      qx -= y * ROPE_R * cx; qy -= y * ROPE_R * cy;
      var sx = quad === 0 || quad === 3 ? -1 : 1, sy = quad < 2 ? -1 : 1, dir = quad === 1 || quad === 3 ? -1 : 1;
      px = sx * qx; py = sy * qy; nx = sx * cx; ny = sy * cy; tx = dir * sx * ux; ty = dir * sy * uy;
    }
    RPT.X = px; RPT.Y = py; RPT.tx = tx; RPT.ty = ty; RPT.nx = nx; RPT.ny = ny;
    return RPT;
  }

  var RR = { z: 0, k: 0, ty: 0, s: 0, y: 0, gap: 1, cov: 1, w: 0, sw: 0, groove: 0 };
  var ropePxr = 0.04, ropeLastX = 0, ropeLastY = 9;                   // one pixel in rope radii, learned from the shader's raster order
  // Which strand is on top at a point, how high, and how near it is to a crease with its neighbour.
  function ropeEval(X, Y, P) {
    ropePos(X, Y, P);
    var s = RP.s, y = RP.y, ph = TAU * s / P.L, best = -9, second = -9, bk = 0, bty = 0, bcw = 1, bgw = 1, cov = 0, k, pxr = ropePxr;
    for (k = 0; k < 3; k++) {
      var al = ph + k * TAU / 3, yk = ROPE_DS * Math.sin(al), dy = y - yk, rs = ROPE_RS * (1 + 0.1 * (vnoise(s * 0.35 + k * 13.7, k * 3.1) - 0.5));
      var c = clamp(0.5 + (rs - Math.abs(dy)) / pxr, 0, 1);
      if (c <= 0) continue;
      if (c > cov) cov = c;
      var z = ROPE_DS * Math.cos(al) + Math.sqrt(Math.max(0, rs * rs - dy * dy));
      if (z > best) { second = best; best = z; bk = k; bty = dy / rs; bcw = smooth(0, 1, (rs - Math.abs(dy)) / (3 * pxr)); bgw = smooth(0, 1, (rs - Math.abs(dy)) / (7 * pxr)); } else if (z > second) second = z;
    }
    var gap = second < -8 ? 1 : best - second, groove = 0;
    if (second > -8) {
      var hh = Math.max(0.14 - gap, 0) / 0.14; best += hh * hh * 0.035;     // creases are rounded a little, as fibre is not a razor edge
      best = second + (best - second) * bcw; groove = 1 - bgw;               // where a strand lies over another, its edge meets the one below over a few pixels, in shadow
    }
    var sw = wrapD(s, P.len), w = 1 - smooth(WHIP_HALF - 0.14, WHIP_HALF, Math.abs(sw));
    RR.s = s; RR.y = y; RR.sw = sw; RR.w = w; RR.k = bk; RR.ty = bty; RR.gap = gap; RR.groove = groove;
    if (best < -8 && w <= 0) { RR.z = -9; return RR; }
    var z0 = best < -8 ? 0 : 1 + best;
    if (w > 0) {                                                       // whipped: smooth twine over the rope, each turn a thin round cord
      var sv = (sw + 0.07 * y) / WHIP_PITCH, tw = fract(sv), cw = clamp(0.5 + (0.97 - Math.abs(y)) / pxr, 0, 1);
      var zw = 1 + Math.sqrt(Math.max(0, 0.94 - y * y)) + (0.055 + 0.025 * lat(Math.floor(sv) & 255, 5)) * bead(0, 1, tw);
      z0 = best < -8 ? zw : z0 + (zw - z0) * w; cov = cov + (cw - cov) * w;
      if (best < -8) { RR.k = 0; RR.gap = 1; }
    }
    RR.z = z0; RR.cov = cov;
    return RR;
  }
  // The yarns on the strand at a point: ridges lying across the strand against its own twist, wandering a little as hand-spun yarn does.
  var RY = { th: 0, om: 0, yarn: 0, yid: 0, a: 0 };
  function ropeYarn(r) {
    var th = Math.asin(clamp(r.ty, -0.97, 0.97)), om = TAU * r.s / ROPE_YARN - ROPE_KAPPA * th + r.k * 2.1 + (vnoise(r.s * 0.55 + r.k * 9.1, 5.5) - 0.5) * 1.6;
    RY.th = th; RY.om = om; RY.yid = Math.floor(om / TAU); RY.yarn = Math.pow(0.5 + 0.5 * Math.cos(om), 1.3);
    RY.a = 0.8 + 0.4 * lat(RY.yid & 255, r.k * 7 + 3);
    return RY;
  }
  var ROPE_TONE = [[1.04, 1.0, 0.95], [0.9, 0.88, 0.84], [1.08, 1.07, 1.02]];
  var ROPE_MAT = {
    width: ROPE_W, depth: 0.8, shadow: 0, ao: true, selfShadow: true,
    height: function (u, X, Y, A, rr) {
      if (Y < ropeLastY) { ropeHW = rr - X; ropeHH = rr - Y; }           // a pass starts at the top-left pixel, whose place gives the frame's half sizes
      if (Y === ropeLastY && X > ropeLastX && X - ropeLastX < 0.01) ropePxr = (X - ropeLastX) / ROPE_R;
      ropeLastX = X; ropeLastY = Y;
      var P = ropePathFor(this.isRound), r = ropeEval(X, Y, P);
      if (r.z < -8 || r.cov <= 0) return NaN;
      var z = r.z + (fbm(X * 9 + 3, Y * 9, 3) - 0.5) * 0.05;
      if (r.w < 1) {
        var y = ropeYarn(r), env = (1 - 0.5 * r.ty * r.ty) * (1 - r.w) * smooth(0.0, 0.22, r.gap);
        z += (0.05 * y.yarn * y.a + 0.016 * Math.cos(3 * y.om + 1.7 * y.th) * bl(110)) * env;
        z += (vnoise(y.om * 0.9 + r.k * 17, y.th * 5 + r.s * 0.8) - 0.5) * 0.04 * bl(170) * env;
        z += (vnoise(y.om * 2.3 + r.k * 5, y.th * 11 + r.s * 1.7) - 0.5) * 0.02 * bl(240) * env;
      }
      return z / 2.1;
    },
    color: function (s) {
      var P = ropePathFor(this.isRound), r = ropeEval(s.X, s.Y, P), N = s.N, k = r.k, X = s.X, Y = s.Y, i;
      var y = ropeYarn(r), occ = clamp(s.occ * 5, 0, 1), crease = Math.max(Math.exp(-r.gap / 0.13), r.groove * 0.7), top = smooth(1.3, 1.95, r.z), side = smooth(0.55, 1, Math.abs(r.y));
      // Hemp: a pale straw with a green-grey cast, bleached on top, dirtier and browner low down and in the grooves.
      var along = fbm(r.s * 0.15 + 3, k * 5.3, 3), tone = ROPE_TONE[k], yt = 0.92 + 0.18 * (lat(y.yid & 255, k + 9) - 0.5) * 2;
      var alb = [196 * tone[0], 162 * tone[1], 112 * tone[2]];
      alb = mix3(alb, [238, 216, 172], top * (0.55 + 0.4 * along));
      alb = mix3(alb, [118, 88, 54], side * 0.55 + smooth(0.55, 0.3, along) * 0.35);
      alb = mix3(alb, [48, 36, 24], clamp(crease * 0.8 + occ * 0.5, 0, 1) * 0.85);
      var fib = vnoise(y.om * 0.9 + k * 17, y.th * 5 + r.s * 0.8), fib2 = vnoise(y.om * 2.3 + k * 5, y.th * 11 + r.s * 1.7);
      var fine = 0.82 + 0.3 * fib * bl(170) + 0.2 * fib2 * bl(240) + 0.1 * (vnoise(X * 260, Y * 260) - 0.5) * bl(260);
      var lum = (0.22 + 1.05 * s.diff) * (1 - 0.7 * occ) * (1 - 0.72 * crease) * (0.84 + 0.3 * y.yarn) * yt * fine;
      var sp = specular(N, 14) * 0.1 * s.lit * (0.5 + fib);
      var out = [alb[0] * lum + 255 * sp, alb[1] * lum + 240 * sp, alb[2] * lum + 215 * sp];
      if (r.w > 0.02) {                                                // the whipping: tarred twine, near black, a little glossy
        var sv = (r.sw + 0.07 * r.y) / WHIP_PITCH, tw = fract(sv), tl = (0.1 + 0.95 * s.diff) * (1 - 0.5 * occ) * (0.7 + 0.5 * bead(0, 1, tw));
        var tsp = specular(N, 30) * 0.25 * s.lit, tt = 0.85 + 0.3 * lat(Math.floor(sv) & 255, 21);
        for (i = 0; i < 3; i++) out[i] = out[i] * (1 - r.w) + ([78, 60, 44][i] * tl * tt + 210 * tsp) * r.w;
      }
      out.push(Math.min(1, r.cov / 0.6));
      return out;
    }
  };

  // Loose fibres: hair-thin strokes standing off the rope, thickest at its edges and along the top, with tufts where the whipping meets the strands.
  // Drawn once at the master size in the rope's own colours and recoloured with it.
  function ropeFuzzDraw(w, h, isRound) {
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), bs = Math.min(w, h), P = ropePathFor(isRound), seed = 11, i;
    function rnd() { seed = (seed * 16807) % 2147483647; return seed / 2147483647; }
    function hair(s, y, ang, len, lit) {
      var pt = ropePoint(P, s, y), tx = pt.tx, ty = pt.ty, nx = pt.nx, ny = pt.ny;
      var dx = (tx * Math.cos(ang) + nx * Math.sin(ang)), dy = (ty * Math.cos(ang) + ny * Math.sin(ang)), L = len * ROPE_R * bs;
      var x0 = w / 2 + pt.X * bs, y0 = h / 2 + pt.Y * bs, bend = (rnd() - 0.5) * 0.9;
      var cxp = x0 + dx * L * 0.5 - dy * L * bend * 0.5, cyp = y0 + dy * L * 0.5 + dx * L * bend * 0.5, x1 = x0 + dx * L + dy * L * bend * 0.3, y1 = y0 + dy * L - dx * L * bend * 0.3;
      var face = nx * LIGHT[0] + ny * LIGHT[1], k = clamp((0.35 + 0.65 * (0.5 - 0.5 * face)) * lit, 0, 1);
      var c = mix3([96, 72, 46], [236, 214, 170], k);
      g.strokeStyle = rgba(c, 0.2 + 0.4 * rnd()); g.lineWidth = Math.max(0.5, bs * (0.0006 + 0.0006 * rnd()));
      g.beginPath(); g.moveTo(x0, y0); g.quadraticCurveTo(cxp, cyp, x1, y1); g.stroke();
    }
    var n = Math.round(P.len * 20);
    for (i = 0; i < n; i++) {                                         // stray fibres, mostly leaving the edges and the crests
      var edge = rnd() < 0.7, sgn = rnd() < 0.5 ? -1 : 1, yy = edge ? sgn * (0.88 + 0.2 * rnd()) : (rnd() - 0.5) * 1.6;
      var ang = (edge ? sgn * (0.2 + 0.9 * rnd()) : (rnd() - 0.5) * 0.5) + (rnd() - 0.5) * 1.2, ln = 0.08 + 0.34 * rnd() * rnd();
      hair(rnd() * P.len, yy, ang, yy > 0 && edge ? ln * 0.6 : ln, 0.5 + 0.7 * rnd());
    }
    for (i = 0; i < 120; i++) {                                       // the ends of the strands, fraying out past the whipping
      var side = i % 2 ? 1 : -1, sTuft = side * (WHIP_HALF + 0.04 + 0.1 * rnd()), yy2 = (rnd() - 0.5) * 1.7;
      hair(sTuft, yy2, side > 0 ? (rnd() - 0.5) * 1.1 : Math.PI + (rnd() - 0.5) * 1.1, 0.12 + 0.3 * rnd(), 0.8 + 0.4 * rnd());
    }
    return cv;
  }
  // The rope's shadow on the picture and on the map, drawn over the picture with the rope itself cut out of it.
  function ropeUnderDraw(w, h, isRound) {
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), bs = Math.min(w, h), rim = rimLayer('rope', w, h, isRound, ROPE_MAT), d = ROPE_R * bs;
    var sil = document.createElement('canvas'); sil.width = w; sil.height = h;
    var sg = sil.getContext('2d'); sg.drawImage(rim, 0, 0); sg.globalCompositeOperation = 'source-in'; sg.fillStyle = '#000'; sg.fillRect(0, 0, w, h);
    g.filter = 'blur(' + d * 0.16 + 'px)'; g.globalAlpha = 0.5; g.drawImage(sil, d * 0.08, d * 0.12);
    g.filter = 'blur(' + d * 0.45 + 'px)'; g.globalAlpha = 0.34; g.drawImage(sil, d * 0.4, d * 0.55);
    g.filter = 'none'; g.globalAlpha = 1; g.globalCompositeOperation = 'destination-out'; g.drawImage(rim, 0, 0);
    return cv;
  }
  function ropeFront(ctx, G) {
    ctx.drawImage(masterLayer('rope-under|' + G.isRound, G.bw, G.bh, function (w, h) { return ropeUnderDraw(w, h, G.isRound); }), G.ox, G.oy);
    ctx.drawImage(graded('rope-fuzz|' + G.isRound, masterLayer('rope-fuzz|' + G.isRound, G.bw, G.bh, function (w, h) { return ropeFuzzDraw(w, h, G.isRound); })), G.ox, G.oy);
  }

  EXTRA.push({ id: 'rope', name: 'Rope', blurb: 'A thick three-strand hemp rope coiled once round the token, its ends bound with dark twine.',
    key: [176, 146, 98], luma: 0.5, spread: 1, spec: { mat: ROPE_MAT, inset: function (G) { return G.rw * 0.5; }, front: ropeFront } });
  /* ---------- Dragon scales ----------
     A band of hide wrapping the ring. Hard keeled plates lie in staggered rows round it, every one overlapping
     the next in the same direction, like a snake's: each rises from under its neighbour to a free edge that
     stands proud and throws a short shadow on the plate beneath. The band is domed, so the plates turn away and
     shrink toward both edges, and the rows are tallest along the middle. Under the gloss the colour moves
     with the angle of each plate, darker in the overlaps, scratched and worn bright along the keels. */
  var SCALES_W = 0.11, SCALES_TL = 1.62, SCALES_T0 = 1.02;     // band width; scale length and where its tip starts, in pitches
  var SCALES_RW = [0.55, 0.85, 1.2, 0.85, 0.55];                  // row heights, relative: biggest along the middle
  var SCALES_RT = {};
  var SCALES_RED = [[142, 14, 24], [156, 24, 22], [120, 10, 26], [104, 12, 22], [168, 30, 26]];
  function scalesPath(d, isRound) { return isRound ? TAU * (0.5 - d) : 4 * (1 - 2 * d); }
  // Rows from the outer edge in: where each lies, how many plates it holds round the ring and how wide they reach.
  function scalesRows(isRound) {
    var key = isRound ? 'r' : 's';
    if (SCALES_RT[key]) return SCALES_RT[key];
    var tot = 0, u = 0, rows = [], k;
    for (k = 0; k < SCALES_RW.length; k++) tot += SCALES_RW[k];
    for (k = 0; k < SCALES_RW.length; k++) {
      var du = SCALES_RW[k] / tot, dc = (u + du / 2) * SCALES_W, hr = du * SCALES_W, n = Math.max(8, Math.round(scalesPath(dc, isRound) / (hr * 1.2)));
      rows.push({ u1: u + du, dc: dc, hr: hr, n: n, ph: (k & 1) * 0.5 + 0.17 * k, B: hr * 0.66 });
      u += du;
    }
    SCALES_RT[key] = rows;
    return rows;
  }
  // Clockwise position round a square ring as a fraction, measured along the mitred path so rows turn the corners.
  function scalesSquareF(X, Y, d) {
    var side = 1 - 2 * d, s;
    if (Math.abs(Y) >= Math.abs(X)) s = Y < 0 ? X + 0.5 - d : 2 * side + 0.5 - d - X;
    else s = X > 0 ? side + Y + 0.5 - d : 3 * side + 0.5 - d - Y;
    return s / (4 * side);
  }
  // The plate on top at this point, found from the rows that can reach it; the result is left in SCALES_HIT.
  var SCALES_HIT = { h: 0, k: 0, j: 0, t: 0, b: 0, tl: 1, ein: 0, en: 0, keel: 0, rnd: 0, B: 0, P: 0, any: false };
  function scalesAt(isRound, X, Y, d0) {
    var rows = scalesRows(isRound), K = rows.length, f0 = isRound ? (Math.atan2(Y, X) + Math.PI) / TAU : scalesSquareF(X, Y, d0);
    // The rows wander a little, as a living hide does, instead of running true.
    var d = d0 + 0.0028 * (vnoise(X * 14 + 3, Y * 14) - 0.5), f = f0 + 0.003 * (vnoise(X * 11, Y * 11 + 5) - 0.5);
    var u = d / SCALES_W, k0 = 0, best = -1, k, jj;
    while (k0 < K - 1 && u > rows[k0].u1) k0++;
    SCALES_HIT.any = false; SCALES_HIT.h = 0;
    for (k = Math.max(0, k0 - 1); k <= Math.min(K - 1, k0 + 1); k++) {
      var R = rows[k], n = R.n, p = f * n - R.ph, j1 = Math.floor(p);
      for (jj = j1 - 1; jj <= j1 + 1; jj++) {
        var jm = ((jj % n) + n) % n, r1 = lat(jm + 29 * k, 17 + k), r2 = lat(jm + 29 * k, 61 + k), r3 = lat(jm + 29 * k + 5, 101 + k);
        var t = p - (jj + (r1 - 0.5) * 0.34), tl = SCALES_TL * (0.94 + 0.12 * r2);
        if (t < 0 || t > tl) continue;
        var B = R.B * (0.92 + 0.16 * r2), b = (d - R.dc) / B, ab = Math.abs(b);
        if (ab > 1) continue;
        var q = 1.35 + 0.65 * r3, t0 = tl - (SCALES_TL - SCALES_T0), tt = t > t0 ? (t - t0) / (tl - t0) : 0;
        var g = tt > 0 ? Math.pow(Math.pow(tt, q) + Math.pow(ab, q), 1 / q) : ab;     // the tip is an oval, rounder or more pointed on each plate
        g += 0.03 * (vnoise(X * 340 + jm, Y * 340 + k) - 0.5);                       // the free edge is never quite true
        if (g >= 1) continue;
        var ein = (1 - g) * B, tn = t / tl, en = 1 - g, bs = smooth(0.011, 0.018, B);
        var hb = 0.03 + (0.17 + 0.07 * r1) * Math.pow(tn, 1.12);                      // the plate rises from under its neighbour to the free edge
        var kw = 0.016 + 0.01 * r3, keel = (0.6 + 0.6 * r2) * Math.exp(-(b - 0.1 * (r3 - 0.5)) * (b - 0.1 * (r3 - 0.5)) / kw) * smooth(0.14, 0.36, tn) * (1 - smooth(0.7, 0.92, tn));
        var gr = Math.exp(-Math.pow((en - 0.2) / 0.035, 2)) + 0.6 * Math.exp(-Math.pow((en - 0.4) / 0.04, 2));   // growth rings echo the free edge
        var h = hb * smooth(0, 0.0034, ein) + 0.012 * bead(0, 0.011, ein) * smooth(0.4, 0.8, tn) + 0.05 * keel - 0.014 * (1 - ab * ab) * smooth(0.2, 0.7, tn)
          - 0.006 * gr * bs * smooth(0.3, 0.6, tn) + 0.02 * (r2 - 0.5) * b * smooth(0.1, 0.5, tn);   // and each is tilted a little its own way
        if (h > best) { best = h; SCALES_HIT.k = k; SCALES_HIT.j = jm; SCALES_HIT.t = t; SCALES_HIT.b = b; SCALES_HIT.ein = ein; SCALES_HIT.en = en; SCALES_HIT.keel = keel; SCALES_HIT.tl = tl; SCALES_HIT.rnd = r1 * 0.5 + r2 * 0.3 + r3 * 0.2; SCALES_HIT.B = B; SCALES_HIT.P = scalesPath(d, isRound) / n; }
      }
    }
    if (best >= 0) { SCALES_HIT.any = true; SCALES_HIT.h = best; }
    return SCALES_HIT;
  }
  // A rake of three claw marks across a few plates: how deep the scratch is here (0 none, 1 the groove floor).
  var SCALES_SCAR = { x: 0.2, y: -0.405, ang: 0.62, len: 0.11 };
  function scalesScar(X, Y) {
    var C = SCALES_SCAR, ca = Math.cos(C.ang), sa = Math.sin(C.ang), dx = X - C.x, dy = Y - C.y, p = dx * ca + dy * sa, q = -dx * sa + dy * ca, m = 0, i;
    if (Math.abs(p) > C.len * 1.1 || Math.abs(q) > 0.02) return 0;
    for (i = -1; i <= 1; i++) {
      var qq = q - i * 0.0065 + 0.0012 * Math.sin(p * 60 + i * 2), a = C.len * (0.7 + 0.3 * lat(i + 5, 3)), taper = smooth(-a, -a + 0.025, p) * (1 - smooth(a - 0.04, a, p));
      m = Math.max(m, (1 - smooth(0.0004, 0.0017, Math.abs(qq))) * taper);
    }
    return m;
  }
  function scalesDome(u) { return 0.2 * Math.sqrt(Math.max(0, 1 - Math.pow(Math.abs(2 * u - 1), 2.6))); }
  // Thin-film sheen: with the plate tilted a little off the eye's line, a deep red turns gold, then green, then teal.
  var SCALES_FILM = [[0, 0, 0], [230, 142, 34], [110, 164, 62], [44, 140, 150]];
  function scalesFilm(tilt) {
    var x = clamp(tilt, 0, 0.999) * 3, i = Math.floor(x), w = smooth(0, 1, x - i);
    return mix3(SCALES_FILM[i], SCALES_FILM[i + 1], w);
  }
  var SCALES_MAT = {
    width: SCALES_W, depth: 0.8, ao: true, selfShadow: true, shadow: 0.7,
    height: function (u, X, Y, A, r) {
      var s = scalesAt(this.isRound, X, Y, r);
      return scalesDome(u) + s.h + 0.004 * (fbm(X * 150, Y * 150, 2) - 0.5) * bl(150) - 0.03 * scalesScar(X, Y);
    },
    color: function (s) {
      var X = s.X, Y = s.Y, N = s.N, sc = scalesAt(this.isRound, X, Y, s.r), tn = sc.t / sc.tl, ab = Math.abs(sc.b);
      if (!sc.any) return [14, 3, 4];
      var occ = clamp(s.occ * 7, 0, 1), id = sc.j + 13 * sc.k;
      // Colour: each plate its own red, the whole hide drifting between crimson and a rustier red round the ring.
      var base = mix3(SCALES_RED[Math.floor(sc.rnd * 4.99)], SCALES_RED[Math.floor(lat(id, 33) * 4.99)], 0.5);
      var drift = fbm(X * 3.2 + 7, Y * 3.2, 3);
      base = mix3(base, [176, 52, 26], smooth(0.5, 0.8, drift) * 0.45);
      var alb = base.map(function (v) { return v * (0.78 + 0.44 * lat(id, 77)) * (0.88 + 0.24 * drift); });
      alb = alb.map(function (v) { return v * (0.55 + 0.45 * smooth(0.05, 0.9, tn)) * (1 - 0.3 * ab * ab); });
      // The free edge is thinner and lets the colour glow, a little more orange; the keel is worn pale.
      var tipw = smooth(0.78, 1, tn) * (1 - smooth(0, 0.01, sc.ein)) * 0.5;
      alb = mix3(alb, [200, 56, 34], tipw * 0.5);
      // Wear: bare keratin along the keel, and fine scratches that run mostly along the plate.
      var ca = (lat(id, 91) - 0.5) * 0.9, cs = Math.cos(ca), sn = Math.sin(ca), tp = sc.t * sc.P, bp = sc.b * sc.B;
      var sx = tp * cs + bp * sn, sy = -tp * sn + bp * cs;
      var line = 1 - smooth(0.008, 0.03, Math.abs(vnoise(sy * 620 + id * 5, sx * 38 + id) - 0.5));
      var patch = smooth(0.4, 0.7, vnoise(sx * 60 + id * 3, sy * 60 + 4));
      var scr = line * patch * (0.25 + 0.75 * sc.keel + 0.5 * smooth(0, 0.012, sc.ein) * (1 - smooth(0.012, 0.03, sc.ein)));
      var wear = sc.keel * smooth(0.45, 0.75, fbm(X * 60 + id, Y * 60, 2)), scar = scalesScar(X, Y);
      alb = mix3(alb, [196, 104, 80], wear * 0.4);
      alb = mix3(alb, [214, 140, 112], scr * 0.55);
      alb = mix3(alb, [196, 120, 96], scar * 0.7);
      var f2 = Math.max(0, N[0] * 0.64 + N[1] * 0.58 + N[2] * 0.5), h2 = Math.max(0, N[0] * 0.33 + N[1] * 0.3 + N[2] * 0.89);      // a dim fill from low right
      var lum = 0.07 + (0.2 * (0.5 + 0.5 * N[2]) * (1 - occ * 0.85) + 1.0 * s.diff + 0.1 * f2 * (1 - occ)) * (1 - occ * 0.3);
      // Gloss: a sharp highlight, a broad soft one, and a faint reflection of the room (a window up left, a dark floor).
      var Rx = 2 * N[2] * N[0], Ry = 2 * N[2] * N[1], Rz = 2 * N[2] * N[2] - 1;
      var win = Math.pow(Math.max(0, Rx * LIGHT[0] + Ry * LIGHT[1] + Rz * LIGHT[2]), 9) * 0.11 + 0.025 * smooth(-0.6, 0.6, -Ry);
      var spec = (specular(N, 120) * 0.8 + specular(N, 20) * 0.05) * s.lit * (1 - occ * 0.6) * (1 + 0.6 * scr) + Math.pow(h2, 90) * 0.22 * (1 - occ);
      var film = scalesFilm((1 - N[2]) * 5 + 0.25 * (lat(id, 5) - 0.5)), fa = (0.4 * s.diff + 0.7 * spec + 0.08) * 0.55 * smooth(0.02, 0.1, 1 - N[2]) * smooth(0.06, 0.16, s.u) * smooth(0.06, 0.16, 1 - s.u);
      var e = win * 255 * s.lit * (1 - occ);
      return [alb[0] * lum + 255 * spec + film[0] * fa + e * 0.8, alb[1] * lum + 232 * spec + film[1] * fa + e * 0.6, alb[2] * lum + 214 * spec + film[2] * fa + e * 0.6];
    }
  };

  EXTRA.push({ id: 'scales', name: 'Dragon scales', blurb: 'Glossy red dragon hide, hard keeled scales overlapping in rows with a sheen that shifts in the light.',
    key: [130, 30, 26], luma: 0.4, spec: { mat: SCALES_MAT } });

  /* ---------- Ghost mist ----------
     Not smoke: light. Thin, pale strands of mist drifting round the token, glowing faintly from inside and mostly
     see-through, so the picture and the map show through them. Strands stretch along the ring, gather into veils
     in places and all but vanish in others, and the odd tendril trails out over the map. The light is worked out
     once per frame in a small texture and added over the token. */
  var GHOST_N = 256, GHOST_R = 0.425, ghostTex = { cv: null, img: null, q: -1, k: '' }, GHOST_PX = null;
  function ghostPixels() { // the pixels the mist can reach, and where each sits on the ring
    var N = GHOST_N, list = [];
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, dr = Math.hypot(X, Y) - GHOST_R;
      if (dr > -0.12 && dr < 0.075) list.push((y * N + x) * 4, Math.atan2(Y, X), dr);
    }
    return list;
  }
  function ghostField(t) {
    var q = Math.round(t * 30), k = gradeId(), T = ghostTex, N = GHOST_N;
    if (T.q === q && T.k === k) return T.cv;
    if (!T.cv) { T.cv = document.createElement('canvas'); T.cv.width = T.cv.height = N; T.img = T.cv.getContext('2d').createImageData(N, N); GHOST_PX = ghostPixels(); }
    var d = T.img.data, P = GHOST_PX, hot = tc([226, 242, 255]), cold = tc([120, 168, 226]);
    d.fill(0);
    for (var i = 0; i < P.length; i += 3) {
      var o = P[i], a = P[i + 1] + t * 0.045, dr = P[i + 2];
      // Polar coordinates scaled so features stretch round the ring and the noise has no seam.
      var rad = 2.6 + dr * 22, px = Math.cos(a) * rad, py = Math.sin(a) * rad;
      var w1 = qfbm(px * 0.6 + t * 0.05, py * 0.6 - t * 0.04, 2), w2 = qfbm(px * 0.6 + 7.3 - t * 0.035, py * 0.6 + 2.1 + t * 0.045, 2);
      var reach = dr > 0 ? 0.018 + 0.05 * smooth(0.45, 0.8, w1) : 0.028 + 0.06 * smooth(0.45, 0.8, w2);
      var env = Math.exp(-dr * dr / (reach * reach));
      if (env < 0.02) continue;
      // Soft veils of glowing haze, and through them strands: the ridges of smooth, slow noise, so they come out as
      // long soft curves that swell and thin and fade along their length, never froth and never wire.
      var hz = qfbm(px * 0.8 + w1 * 1.6 - t * 0.03, py * 0.8 + w2 * 1.6 + t * 0.02, 3), haze = smooth(0.42, 0.8, hz);
      var n1 = qn(px * 0.85 + w1 * 3.2 + t * 0.06, py * 0.85 + w2 * 3.2 - t * 0.05), r1 = 1 - Math.abs(2 * n1 - 1);
      var n2 = qn(px * 1.5 + w2 * 2.4 - t * 0.08 + 11, py * 1.5 + w1 * 2.4 + 4.2), r2 = 1 - Math.abs(2 * n2 - 1);
      var wid = 3 + 6 * qn(px * 0.5 + 20, py * 0.5 - t * 0.04);
      var veil = smooth(0.32, 0.75, qfbm(px * 0.4 + 3 + t * 0.025, py * 0.4 - t * 0.02, 2)), along = veil * veil;
      var strand = (Math.pow(r1, wid) + 0.45 * Math.pow(r2, wid + 3)) * along;
      var L = env * (haze * 0.45 * (0.35 + 0.65 * veil) + strand * 0.75 + Math.pow(r1, 1.5) * 0.06 * veil);
      if (L < 0.004) continue;
      var m = Math.min(1, strand * 1.2 + haze * 0.2);
      d[o] = cold[0] + (hot[0] - cold[0]) * m; d[o + 1] = cold[1] + (hot[1] - cold[1]) * m; d[o + 2] = cold[2] + (hot[2] - cold[2]) * m;
      d[o + 3] = Math.min(255, L * 330);
    }
    T.cv.getContext('2d').putImageData(T.img, 0, 0); T.q = q; T.k = k;
    return T.cv;
  }
  function ghostFront(ctx, G, t) {
    var S0 = Math.min(G.outW, G.outH), cx = G.outW / 2, cy = G.outH / 2;
    // A faint cold dusk under the mist, so pale strands still read on a pale map.
    var g = ctx.createRadialGradient(cx, cy, S0 * 0.34, cx, cy, S0 * 0.5);
    g.addColorStop(0, 'rgba(8,14,24,0)'); g.addColorStop(0.45, 'rgba(8,14,24,.16)'); g.addColorStop(1, 'rgba(8,14,24,0)');
    ctx.save(); ctx.fillStyle = g; ctx.fillRect(0, 0, G.outW, G.outH);
    ctx.globalCompositeOperation = 'lighter'; ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(ghostField(t), 0, 0, G.outW, G.outH); ctx.restore();
  }

  /* ---------- Poison gas ----------
     Gas, so it behaves like steam: soft, pale veils of it rolling slowly round the ring, with curling wisps
     drawn out of them that thin to nothing, mostly drifting out over the map and a little way in over the
     picture. It is lit from the top left, so each fold of it has a pale side and a shaded side, and it is always
     see-through. It casts a faint shadow on whatever is under it. Worked out once per frame in a small texture. */
  var POISON_N = 224, POISON_R = 0.42, poisonTex = { cv: null, img: null, q: -1, k: '' }, POISON_PX = null, POISON_D = null;
  function poisonPixels() {
    var N = POISON_N, list = [];
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, dr = Math.hypot(X, Y) - POISON_R;
      if (dr > -0.1 && dr < 0.08) list.push(y * N + x, Math.atan2(Y, X), dr);
    }
    return list;
  }
  function poisonField(t) {
    var q = Math.round(t * 30), k = gradeId(), T = poisonTex, N = POISON_N;
    if (T.q === q && T.k === k) return T.cv;
    if (!T.cv) { T.cv = document.createElement('canvas'); T.cv.width = T.cv.height = N; T.img = T.cv.getContext('2d').createImageData(N, N); POISON_PX = poisonPixels(); POISON_D = new Float32Array(N * N); }
    var P = POISON_PX, D = POISON_D, d = T.img.data, i;
    D.fill(0);
    for (i = 0; i < P.length; i += 3) {
      var a = P[i + 1] + t * 0.035, dr = P[i + 2];
      var rad = 2.4 + dr * 16, px = Math.cos(a) * rad, py = Math.sin(a) * rad;
      var w1 = qfbm(px * 0.7 + t * 0.06, py * 0.7 - t * 0.05, 2), w2 = qfbm(px * 0.7 + 7.3 - t * 0.045, py * 0.7 + 2.1 + t * 0.055, 2);
      var reach = dr > 0 ? 0.03 + 0.045 * smooth(0.4, 0.8, w1) : 0.03 + 0.05 * smooth(0.4, 0.8, w2);
      var env = Math.exp(-dr * dr / (reach * reach));
      if (env < 0.03) continue;
      // Veils: soft, folded noise; wisps: the ridges of slow noise, wide and soft, pulled out of the veils.
      var vn = qfbm(px * 1.1 + w1 * 2 - t * 0.04, py * 1.1 + w2 * 2 + t * 0.03, 3), veil = smooth(0.4, 0.75, vn);
      var n1 = qn(px * 0.9 + w1 * 3 + t * 0.07, py * 0.9 + w2 * 3 - t * 0.05), r1 = 1 - Math.abs(2 * n1 - 1);
      var wisp = Math.pow(r1, 4) * smooth(0.3, 0.7, qfbm(px * 0.45 + 5, py * 0.45 - t * 0.03, 2));
      D[P[i]] = Math.min(1, (veil * 0.85 + wisp * 0.6) * env);
    }
    var lo = tc([88, 104, 34]), mid = tc([170, 196, 80]), hi = tc([238, 246, 196]);
    d.fill(0);
    for (i = 0; i < P.length; i += 3) {
      var j = P[i], dn = D[j];
      if (dn <= 0.004) continue;
      // Lit from the top left: pale where the gas rises toward the light, shaded on the far side of each fold.
      var up = D[j - 2 * N - 2] || 0, lit = clamp(0.55 + (dn - up) * 2.4 - dn * 0.25, 0, 1);
      var c = lit < 0.5 ? mix3(lo, mid, lit * 2) : mix3(mid, hi, (lit - 0.5) * 2), o = j * 4;
      d[o] = c[0]; d[o + 1] = c[1]; d[o + 2] = c[2]; d[o + 3] = 255 * Math.min(1, dn * 0.85);
    }
    T.cv.getContext('2d').putImageData(T.img, 0, 0); T.q = q; T.k = k;
    return T.cv;
  }
  function poisonFront(ctx, G, t) {
    var S0 = Math.min(G.outW, G.outH), cx = G.outW / 2, cy = G.outH / 2, R = S0 * POISON_R, shade = tc([20, 30, 12]);
    ctx.save();
    var sg = ctx.createRadialGradient(cx + S0 * 0.008, cy + S0 * 0.012, R - S0 * 0.07, cx + S0 * 0.008, cy + S0 * 0.012, R + S0 * 0.08);
    sg.addColorStop(0, rgba(shade, 0)); sg.addColorStop(0.45, rgba(shade, 0.1)); sg.addColorStop(1, rgba(shade, 0));
    ctx.fillStyle = sg; ctx.fillRect(0, 0, G.outW, G.outH);
    ctx.imageSmoothingQuality = 'high'; ctx.drawImage(poisonField(t), 0, 0, G.outW, G.outH); ctx.restore();
  }
  EXTRA.push({ id: 'ghost', name: 'Ghost mist', blurb: 'Thin strands of pale light drifting round the token, mostly see-through, the odd one trailing away.',
    key: [180, 214, 248], luma: 0.2, spread: 0.6, spec: { anim: true, roundOnly: true, rim: 0.08, feather: 0.04, front: ghostFront } });
  EXTRA.push({ id: 'poison', name: 'Poison gas', blurb: 'Sickly green gas puffing out of the ring like steam, billowing and thinning as it drifts away.',
    key: [170, 196, 80], luma: 0.4, spread: 0.6, spec: { anim: true, roundOnly: true, rim: 0.08, feather: 0.04, front: poisonFront } });
  /* ---------- Stained glass ----------
     A ring of leaded cathedral glass, lit from behind like the border of a rose window. Irregular pieces are cut
     and held by dark lead came: a raised, rounded strip with solder blobs where leads meet. The glass is not lit
     by the room; its colour is light coming through it, so it is worked out here from how thick the piece is
     (uneven, streaky, with seed bubbles and a little painted grisaille) and the lead, the only thing the room
     lights, gets the real relief. */
  var STAINEDGLASS_W = 0.11, STAINEDGLASS_B = 0.0095, STAINEDGLASS_HW = 0.0048;   // band width, border lead width, half the width of an inner lead
  var STAINEDGLASS_PAL = [
    { c: [16, 40, 176], a: [96, 150, 232] },     // cobalt blue, streaked with sky
    { c: [182, 8, 30], a: [238, 100, 38] },      // ruby, streaked orange
    { c: [240, 150, 18], a: [255, 214, 84] },    // amber
    { c: [14, 124, 58], a: [160, 206, 60] },     // emerald, streaked yellow-green
    { c: [214, 230, 218], a: [250, 248, 238] },  // pale glass
    { c: [26, 84, 182], a: [150, 200, 226] }     // a lighter blue
  ];
  var STAINEDGLASS_BAG = [0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 4, 5, 5];     // palette indices: how often each colour is drawn
  var STAINEDGLASS_GRID = {};
  function stainedglassPath(d, isRound) { return isRound ? TAU * (0.5 - d) : 4 * (1 - 2 * d); }
  // Clockwise position round a square ring as a fraction, measured along the mitred path so the pieces turn the corners.
  function stainedglassSquareF(X, Y, d) {
    var side = 1 - 2 * d, s;
    if (Math.abs(Y) >= Math.abs(X)) s = Y < 0 ? X + 0.5 - d : 2 * side + 0.5 - d - X;
    else s = X > 0 ? side + Y + 0.5 - d : 3 * side + 0.5 - d - Y;
    return s / (4 * side);
  }
  // The pieces round the ring: columns of one tall piece, two or three short ones, each with its own colour, kept
  // different from its neighbours, and its own character (streaky, seedy, painted).
  function stainedglassGrid(isRound) {
    var key = isRound ? 'r' : 's';
    if (STAINEDGLASS_GRID[key]) return STAINEDGLASS_GRID[key];
    var n = Math.round(stainedglassPath(STAINEDGLASS_W / 2, isRound) / 0.066), cols = [], i, sub, last = -1, seed = 7, bag = [];
    function rnd() { seed = (seed * 16807) % 2147483647; return seed / 2147483647; }
    for (i = 0; i < n; i++) {
      var r0 = rnd(), kind = r0 < 0.34 ? 1 : r0 < 0.86 ? 2 : 3, ys = kind === 1 ? [0.5 + 0.3 * (rnd() - 0.5)] : kind === 2 ? [0.2 + 0.2 * rnd(), 0.6 + 0.2 * rnd()] : [0.16 + 0.06 * rnd(), 0.46 + 0.08 * rnd(), 0.8 + 0.06 * rnd()];
      var c = { x: (i + 0.5 + 0.85 * (rnd() - 0.5)) / n, p: [] };
      for (sub = 0; sub < kind; sub++) {
        var pick, tries = 0, at;
        // Colours come out of a shuffled bag, so the mix stays even all the way round, and never the same twice running.
        if (!bag.length) { bag = STAINEDGLASS_BAG.slice(); for (var sh = bag.length - 1; sh > 0; sh--) { var sw = Math.floor(rnd() * (sh + 1)), tmp = bag[sh]; bag[sh] = bag[sw]; bag[sw] = tmp; } }
        do { at = Math.floor(rnd() * bag.length); pick = bag[at]; tries++; } while ((pick === last || (sub > 0 && pick === c.p[sub - 1].k)) && tries < 8 && bag.length > 1);
        bag.splice(at, 1);
        last = pick;
        c.p.push({ k: pick, y: ys[sub], ang: rnd() * TAU, seedy: rnd() < 0.4, hammered: rnd() < 0.2, opal: rnd() < 0.1 && pick !== 1 && pick !== 4, paint: rnd() < 0.24 ? 1 + Math.floor(rnd() * 2.99) : 0,
          streak: rnd(), hue: rnd(), tone: rnd(), s: rnd() * 100 });
      }
      cols.push(c);
    }
    STAINEDGLASS_GRID[key] = { n: n, cols: cols };
    return STAINEDGLASS_GRID[key];
  }
  var STAINEDGLASS_SITES = [], STAINEDGLASS_Q = { piece: null, e1: 0, e2: 0, ex: 0, ey: 0, eb: 0, bt: 2, hw: 0 };
  // Where this point sits: which piece, how far from the nearest lead, and the profile of the lead and solder.
  function stainedglassAt(isRound, X, Y, d) {
    var G = stainedglassGrid(isRound), n = G.n, f = isRound ? (Math.atan2(Y, X) + Math.PI) / TAU : stainedglassSquareF(X, Y, d), Q = STAINEDGLASS_Q;
    var Lp = stainedglassPath(d, isRound), zone = STAINEDGLASS_W - 2 * STAINEDGLASS_B, c0 = Math.floor(f * n), S = STAINEDGLASS_SITES, ns = 0, i, k;
    // Hand-cut glass and hand-bent lead are never straight: the whole pattern wanders by about a pixel.
    var wx = 0.0026 * (vnoise(X * 34 + 5, Y * 34) - 0.5), wy = 0.0026 * (vnoise(X * 34, Y * 34 + 9) - 0.5);
    for (i = c0 - 2; i <= c0 + 2; i++) {
      var im = ((i % n) + n) % n, col = G.cols[im];
      for (k = 0; k < col.p.length; k++) {
        var p = col.p[k], site = S[ns] || (S[ns] = { x: 0, y: 0, p: null });
        site.x = (col.x + (i - im) / n - f) * Lp - wx;     // along the ring, at this point's own radius
        site.y = STAINEDGLASS_B + p.y * zone - d - wy;
        site.p = p; ns++;
      }
    }
    var best = 0, bd = 1e9, s;
    for (k = 0; k < ns; k++) { s = S[k]; var dd = s.x * s.x + s.y * s.y; if (dd < bd) { bd = dd; best = k; } }
    var s1 = S[best], e1 = 1e9, e2 = 1e9;
    for (k = 0; k < ns; k++) {
      if (k === best) continue;
      s = S[k];
      var vx = s.x - s1.x, vy = s.y - s1.y, e = (s.x * s.x + s.y * s.y - bd) / (2 * Math.sqrt(vx * vx + vy * vy));
      if (e < e1) { e2 = e1; e1 = e; } else if (e < e2) e2 = e;
    }
    // The border leads run along both edges of the band; a piece's own edge distance includes them.
    var eb = Math.min(d + wy - STAINEDGLASS_B, STAINEDGLASS_W - STAINEDGLASS_B - d - wy);
    var hwb = (STAINEDGLASS_B + STAINEDGLASS_HW) / 2, dd2 = d + wy;
    Q.piece = s1.p; Q.ex = -s1.x; Q.ey = -s1.y; Q.e1 = e1; Q.e2 = e2; Q.eb = eb;
    Q.bt = eb < STAINEDGLASS_HW ? Math.abs((dd2 < STAINEDGLASS_W / 2 ? dd2 : STAINEDGLASS_W - dd2) - hwb) / hwb : 2;      // across the border lead: 0 on its crown, 1 at its foot
    Q.hw = STAINEDGLASS_HW * (0.82 + 0.36 * vnoise(X * 50 + 2, Y * 50 + 8));       // and varies in width as it was drawn through the mill
    return Q;
  }
  // How the lead stands: a round-topped strip, the border leads broader than the inner ones.
  function stainedglassLead(q) {
    var t1 = q.e1 / q.hw, a = t1 < 1 ? Math.pow(1 - t1 * t1, 0.8) : 0, b = q.bt < 1 ? Math.pow(1 - q.bt * q.bt, 0.7) : 0;
    return a > b ? a : b;
  }
  // Solder: a flat blob where leads meet, found where two lead edges are both close.
  function stainedglassSolder(q) {
    var j = Math.min(Math.hypot(q.e1, q.e2), Math.hypot(q.e1, q.eb - q.hw * 0.5));
    return 1 - smooth(0.0016, 0.0056, j);
  }
  // Glass thickness: streaks along a direction of the piece's own, broken up by slow swirls, with fine ream lines
  // where it was drawn out. Thicker glass is darker and richer; thinner passes more light.
  var STAINEDGLASS_T = { tau: 0.5, st: 0.5 };
  function stainedglassTau(pc, X, Y, ex, ey) {
    var ca = Math.cos(pc.ang), sa = Math.sin(pc.ang), x1 = X * ca + Y * sa, y1 = -X * sa + Y * ca;
    var w = fbm(X * 7 + pc.s, Y * 7, 3), st = fbm(x1 * 9 + w * 2.2 + pc.s, y1 * 24 + pc.s * 1.3, 4), sw = fbm(X * 20 + w * 3 + 11, Y * 20 + pc.s, 3);
    var ream = 0.5 + 0.5 * Math.sin((y1 * 52 + w * 14 + sw * 3) * TAU / 2);
    var ramp = (ex * Math.cos(pc.ang * 2) + ey * Math.sin(pc.ang * 2)) / 0.03;               // glass drawn thicker on one side of the piece
    var t = 0.46 * st + 0.4 * sw + 0.14 * ream;
    STAINEDGLASS_T.st = st; STAINEDGLASS_T.tau = clamp(0.5 + (t - 0.5) * 2.4 + 0.22 * ramp, 0, 1);
    return STAINEDGLASS_T;
  }
  // A seed bubble: a small ball of air, bright on the side facing the light and dark on the far rim.
  function stainedglassBubble(X, Y, dens) {
    var cs = 0.03, cx = Math.floor(X / cs), cy = Math.floor(Y / cs);
    if (lat(cx, cy + 40) > dens) return 0;
    var bx = (cx + 0.25 + 0.5 * lat(cx + 7, cy)) * cs, by = (cy + 0.25 + 0.5 * lat(cx, cy + 9)) * cs, R = 0.0014 + 0.0026 * lat(cx + 3, cy + 5);
    var ux = (X - bx) / R, uy = (Y - by) * 1.2 / R, r = Math.sqrt(ux * ux + uy * uy);
    if (r >= 1.1) return 0;
    return (-(ux + uy) * 0.7 * smooth(0.3, 0.95, r) + 0.12 * (1 - r) - 0.25 * smooth(0.85, 1, r)) * (1 - smooth(1, 1.1, r));
  }
  // Grisaille: matt paint fired onto a piece. One kind is a soft wash with a few brushed strokes, each starting and
  // ending where the brush did; another is a stippled matting that darkens from one side; the third a loose scroll.
  function stainedglassPaint(q, e, hw) {
    var pc = q.piece, ex = q.ex, ey = q.ey, m = 0;
    if (!pc.paint) return 0;
    if (pc.paint === 1) {
      var ca = Math.cos(pc.ang), sa = Math.sin(pc.ang), across = ex * ca + ey * sa, along = -ex * sa + ey * ca;
      var ph = across / 0.0085 + 0.8 * vnoise(along * 30, across * 5), li = Math.floor(ph), fr = Math.abs(ph - li - 0.5) * 2;
      var c = (lat(li & 255, 21) - 0.5) * 0.03, hl = 0.008 + 0.022 * lat(li & 255, 55), taper = 1 - smooth(0.4, 1, Math.abs(along - c) / hl), w = 0.5 + 0.5 * lat(li & 255, 9);
      m = (1 - smooth(w * 0.1, w * 0.6, fr)) * taper * 0.75 * smooth(0.3, 0.5, vnoise(ex * 24 + pc.s, ey * 24));
      m = Math.max(m, (1 - smooth(0, 0.02, e - hw)) * smooth(0.3, 0.7, vnoise(ex * 22 + pc.s, ey * 22)) * 0.35);
    } else if (pc.paint === 2) {
      var side = (ex * Math.cos(pc.ang) + ey * Math.sin(pc.ang)) / 0.03 + 0.5, grain = vnoise(ex * 210 + pc.s, ey * 210);
      m = smooth(0.15, 0.95, side) * smooth(0.28, 0.62, grain) * 0.55 * bl(210);
    } else {
      var f = fbm(ex * 30 + pc.s, ey * 30, 2);
      m = (1 - smooth(0.008, 0.03, Math.abs(f - 0.5))) * smooth(0.4, 0.58, vnoise(ex * 14 + pc.s, ey * 14 + 3)) * 0.72;
    }
    return clamp(m, 0, 1);
  }
  var STAINEDGLASS_MAT = {
    width: STAINEDGLASS_W, depth: 0.5, ao: true, selfShadow: true, shadow: 0.7,
    height: function (u, X, Y, A, r) {
      var q = stainedglassAt(this.isRound, X, Y, r), e = Math.min(q.e1, q.eb), pc = q.piece;
      var gh = 0.1 + 0.05 * stainedglassTau(pc, X, Y, q.ex, q.ey).tau;
      if (pc.hammered) { var hv = voronoi(X * 55 + pc.s, Y * 55); gh += 0.02 * Math.pow(1 - clamp(hv.f1 / 0.6, 0, 1), 2) * bl(55); }
      return gh + 0.27 * stainedglassLead(q) + 0.07 * Math.sqrt(stainedglassSolder(q));
    },
    color: function (s) {
      var X = s.X, Y = s.Y, N = s.N, q = stainedglassAt(this.isRound, X, Y, s.r), hw = q.hw, e = Math.min(q.e1, q.eb), pc = q.piece;
      var pal = STAINEDGLASS_PAL[pc.k], T = stainedglassTau(pc, X, Y, q.ex, q.ey), sol = stainedglassSolder(q), lead = stainedglassLead(q);
      var gl, ge = e - hw;
      if (lead < 0.98) {
        // Glass: transmitted light, brighter in the middle of the piece, darker where thick and toward the lead.
        var am = smooth(0.55, 0.78, T.st) * (0.25 + 0.75 * pc.streak) * 0.75, tint = 0.9 + 0.2 * pc.hue;
        var c0 = mix3(pal.c, pal.a, am), gam = (0.34 + 1.5 * T.tau) * (0.85 + 0.3 * pc.hue);
        if (pc.opal) { c0 = mix3(c0, [244, 238, 226], 0.25 + 0.55 * smooth(0.35, 0.8, T.st)); gam = 0.5 + 0.7 * T.tau; }       // opalescent glass is milky and swirled
        var g = clamp(ge / 0.016, 0, 1), vig = (0.66 + 0.34 * Math.pow(g, 0.8)) * (0.78 + 0.4 * pc.tone), lens = 1 + 0.3 * clamp(s.curv * 2, -0.4, 0.5);
        var back = 0.86 + 0.34 * fbm(X * 2.6 + 3, Y * 2.6, 3);                                 // the sky behind is never even
        var hot = g * g * 0.13, soil = (1 - smooth(0, 0.012, ge)) * smooth(0.5, 0.78, fbm(X * 44 + 3, Y * 44, 3)) * 0.35;
        gl = c0.map(function (v, k) { var t = clamp(v * tint / 255, 0, 1); return (255 * Math.pow(t, gam) * vig * lens * back + (pal.a[k] * hot)) * (1 - soil); });
        var bub = pc.seedy || lat(Math.floor(X * 33), Math.floor(Y * 33)) < 0.05 ? stainedglassBubble(X, Y, pc.seedy ? 0.42 : 0.04) : 0;
        if (bub) gl = gl.map(function (v) { return bub > 0 ? v * (1 + bub * 0.6) + 80 * bub * bub : v * (1 + bub * 0.9); });
        var pm = stainedglassPaint(q, e, hw);
        if (pm > 0) gl = gl.map(function (v, k) { return v * (1 - 0.9 * pm) + [20, 12, 8][k] * pm; });
        var occ = clamp(s.occ * 6, 0, 1), glow = (0.9 + 0.1 * s.lit) * (1 - occ * 0.25), spg = specular(N, 40) * 70 + 9 * smooth(-0.6, 0.5, -(X + Y) * 1.2 - N[0] * 3 - N[1] * 3);
        gl = [gl[0] * glow + spg, gl[1] * glow + spg, gl[2] * glow + spg].map(function (v) { return v < 170 ? v : 170 + 85 * (1 - Math.exp(-(v - 170) / 85)); });
        if (lead <= 0.02 && sol <= 0.02) return gl;
      }
      // Lead: matte, dark and a little oxidised, a faint highlight along its crown; solder is paler and smoother.
      var ox = smooth(0.55, 0.8, fbm(X * 30 + 5, Y * 30, 3)), sc = 1 - smooth(0.012, 0.04, Math.abs(vnoise(X * 700 + 3, Y * 90) - 0.5)) * smooth(0.5, 0.7, vnoise(X * 40, Y * 40));
      var lum = (0.16 + s.diff * 0.9) * (0.9 + 0.2 * vnoise(X * 160, Y * 160) * bl(160)), sp = specular(N, 16) * 20 + sc * 16, lb = mix3([40, 42, 46], [84, 86, 86], ox * 0.7);
      var lc = [lb[0] * lum + sp, lb[1] * lum + sp, lb[2] * lum + sp * 1.05];
      if (sol > 0.05) { var sl = 0.28 + s.diff * 0.85, ss = specular(N, 30) * 80; lc = mix3(lc, [112 * sl + ss, 114 * sl + ss, 120 * sl + ss], smooth(0.05, 0.5, sol)); }
      if (!gl) return lc;
      var m = smooth(0.02, 0.5, Math.max(lead, sol));
      return [gl[0] + (lc[0] - gl[0]) * m, gl[1] + (lc[1] - gl[1]) * m, gl[2] + (lc[2] - gl[2]) * m];
    }
  };

  // Light spilling from the glass: a tight blur of the ring added back over it, so the colour bleeds a little onto the
  // lead, and a wide one cast onto the picture's edge. Both stay inside the token.
  var STAINEDGLASS_BLOOM = new Map();
  function stainedglassBlur(rim, G, radius, tag) {
    var key = tag + '|' + G.bw + 'x' + G.bh + '|' + G.isRound + '|' + gradeId(), b = STAINEDGLASS_BLOOM.get(key);
    if (b && b.src === rim) return b.cv;
    if (STAINEDGLASS_BLOOM.size > 60) STAINEDGLASS_BLOOM.clear();
    var cv = document.createElement('canvas'), g = cv.getContext('2d');
    cv.width = G.bw; cv.height = G.bh; g.filter = 'blur(' + Math.max(1, G.bs * radius) + 'px)'; g.drawImage(rim, 0, 0);
    STAINEDGLASS_BLOOM.set(key, { src: rim, cv: cv });
    return cv;
  }
  function stainedglassFront(ctx, G) {
    var rim = graded('rim|stainedglass', rimLayer('stainedglass', G.bw, G.bh, G.isRound, STAINEDGLASS_MAT));
    ctx.save(); ctx.translate(G.ox, G.oy); shapePath(ctx, G.bw, G.bh, 0, G.isRound); ctx.clip();
    ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = 0.26; ctx.drawImage(stainedglassBlur(rim, G, 0.016, 'near'), 0, 0);
    ctx.restore(); ctx.save(); ctx.translate(G.ox, G.oy); shapePath(ctx, G.bw, G.bh, STAINEDGLASS_W * G.bs * 0.9, G.isRound); ctx.clip();
    ctx.globalCompositeOperation = 'lighter'; ctx.globalAlpha = 0.3; ctx.drawImage(stainedglassBlur(rim, G, 0.05, 'far'), 0, 0); ctx.restore();
  }

  EXTRA.push({ id: 'stainedglass', name: 'Stained glass', blurb: 'Leaded cathedral glass lit from behind, jewel colours between dark lead came.',
    key: [40, 80, 170], luma: 0.2, spread: 1, spec: { mat: STAINEDGLASS_MAT, front: stainedglassFront } });
  /* ---------- Thunderstorm ----------
     A bank of storm cloud seen from above, painted the way a cumulus tower is built: heads of cloud heaped round
     the ring, each one a cluster of rounded billows lit from above with soft, wrapped light, so the tops stand
     bright while the undersides and the gaps between heads fall into blue-grey shade. A head throws a soft shade on
     the heads below it, and its edge frays into wisps, so nothing is outlined and nothing has a seam. Each head is
     worked out once as a sprite (its shape and its light, grade-free) and recoloured per grade; the heads are laid
     back to front, roll round the ring at slightly different speeds, and swell and subside as they go. Lightning
     lights the heads near it from within, and bolts are drawn over everything as sharp strokes. The painting is
     done once per frame and size, small enough to be cheap, and drawn scaled into every canvas of that size. */
  var STORM_SC = 0.88, STORM_SP = 128, STORM_ROT = 0.05, STORM_NV = 12, STORM_RMIN = 0.322, STORM_RMAX = 0.49;
  var STORM_EXPO = [0.4, 0.5, 0.62, 0.76, 0.92, 1.08, 1.25];       // the exposures a head can be drawn at
  // The sun, from the upper left and fairly low, so every billow has a lit and a shaded side.
  var STORM_SUN = (function () { var v = [-0.45, -0.75, 0.5], n = Math.hypot(v[0], v[1], v[2]); return [v[0] / n, v[1] / n, v[2] / n]; })();
  var stormSprites = null, stormP = null, stormTex = {}, stormSets = {};
  // exp(-x) from a table: the glow needs it for every head every frame
  var STORM_EXP = (function () { var a = new Float32Array(1025); for (var i = 0; i <= 1024; i++) a[i] = Math.exp(-i / 128); return a; })();
  function stormExp(x) { var i = (x * 128) | 0; return i > 1023 ? 0 : STORM_EXP[i]; }
  function stormRand(seed) { var s = (seed * 2654435761 + 12345) >>> 0; return function () { s = (Math.imul(s, 1664525) + 1013904223) >>> 0; return s / 4294967296; }; }

  /* ---------- A head of cloud, as a sprite ---------- */
  // Billows are spheres heaped into a height field with a soft maximum, so where two meet there is a rounded
  // crease and not a seam; smaller billows sit on the edges of the bigger ones. Lit by a wrapped diffuse term
  // (cloud scatters light into its shade), darkened in the creases and under the lee of a taller billow, and
  // broken up by a soft cottony noise. The outline is feathered and frays into a halo of wisps.
  function stormSprite(seed) {
    var S = STORM_SP, n = S * S, R = stormRand(seed * 17 + 3), x, y, i, k, u, v;
    var UU = new Float32Array(n), VV = new Float32Array(n), H = new Float32Array(n);
    for (y = 0; y < S; y++) for (x = 0; x < S; x++) {
      u = (x + 0.5) / S * 2 - 1; v = (y + 0.5) / S * 2 - 1; i = y * S + x;
      UU[i] = u + (qfbm(u * 2.2 + seed * 3.1, v * 2.2 + 5, 3) - 0.5) * 0.16;
      VV[i] = v + (qfbm(u * 2.2 + 9, v * 2.2 + seed * 2.3, 3) - 0.5) * 0.16;
    }
    var R0 = 0.5, L = [[0.05 * (R() - 0.5), 0.05 * (R() - 0.5), R0, 0]];
    for (k = 0; k < 8; k++) { var an = (k + R() * 0.8) / 8 * TAU, dd = R0 * (0.55 + 0.5 * R()), rr = R0 * (0.42 + 0.35 * R()); L.push([Math.cos(an) * dd, Math.sin(an) * dd, rr, -0.25 * R0 * R()]); }
    for (k = 0; k < 12; k++) { var b = L[Math.floor(R() * 9)], a2 = R() * TAU, d2 = b[2] * (0.6 + 0.4 * R()), r2 = R0 * (0.16 + 0.14 * R()); L.push([b[0] + Math.cos(a2) * d2, b[1] + Math.sin(a2) * d2, r2, b[3] + b[2] * 0.3 - r2 * 0.3]); }
    var K = 0.05;
    L.forEach(function (lb) {
      var m = lb[2] + 0.12, xa = Math.max(0, Math.floor((lb[0] - m + 1) / 2 * S)), xb = Math.min(S - 1, Math.ceil((lb[0] + m + 1) / 2 * S));
      var ya = Math.max(0, Math.floor((lb[1] - m + 1) / 2 * S)), yb = Math.min(S - 1, Math.ceil((lb[1] + m + 1) / 2 * S));
      for (y = ya; y <= yb; y++) for (x = xa; x <= xb; x++) {
        i = y * S + x;
        var dx = UU[i] - lb[0], dy = VV[i] - lb[1], q2 = (dx * dx + dy * dy) / (lb[2] * lb[2]);
        if (q2 >= 1) continue;
        var cap = lb[3] + lb[2] * 0.9 * Math.sqrt(1 - q2), h = H[i], df = cap - h, ad = df < 0 ? -df : df;
        H[i] = (cap > h ? cap : h) + (ad < K ? (K - ad) * (K - ad) / (4 * K) : 0);
      }
    });
    for (i = 0; i < n; i++) H[i] += 0.02 * (qfbm(UU[i] * 9 + seed * 5, VV[i] * 9 + 2, 3) - 0.5) * smooth(0.02, 0.15, H[i]);   // the cauliflower grain of a cloud's surface
    H = boxBlur(H, S, S, 1);
    var C = new Float32Array(n);
    for (i = 0; i < n; i++) C[i] = smooth(0, 0.16, H[i]);
    C = boxBlur(C, S, S, 2);
    var Hb = boxBlur(H, S, S, 5), halo = boxBlur(C, S, S, 7), lum = new Float32Array(n), al = new Float32Array(n), lg = new Float32Array(n), ll = new Float32Array(n);
    var sc = S / 2 * 1.0, W = 0.35, Lx = STORM_SUN[0], Ly = STORM_SUN[1], Lz = STORM_SUN[2], hy = Math.hypot(Lx, Ly), tap = [3, 6, 10, 15], tx = [], ty = [], tr = [];
    for (k = 0; k < 4; k++) { tx.push(Math.round(Lx / hy * tap[k])); ty.push(Math.round(Ly / hy * tap[k])); tr.push(tap[k] / S * 2 * Lz / hy); }
    for (y = 1; y < S - 1; y++) for (x = 1; x < S - 1; x++) {
      i = y * S + x; u = (x + 0.5) / S * 2 - 1; v = (y + 0.5) / S * 2 - 1;
      var gx = (H[i + 1] - H[i - 1]) * 0.5 * sc, gy = (H[i + S] - H[i - S]) * 0.5 * sc, nn = 1 / Math.sqrt(gx * gx + gy * gy + 1);
      var diff = (-gx * nn * Lx - gy * nn * Ly + nn * Lz + W) / (1 + W); diff = diff < 0 ? 0 : diff > 1 ? 1 : diff;
      var occ = (Hb[i] - H[i]) * 7, sh = 0; occ = occ < 0 ? 0 : occ > 1 ? 1 : occ;
      for (k = 0; k < 4; k++) {
        var xs = Math.min(S - 1, Math.max(0, x + tx[k])), ys = Math.min(S - 1, Math.max(0, y + ty[k])), ov = (H[ys * S + xs] - H[i] - tr[k]) * 8;
        if (ov > sh) sh = ov;
      }
      if (sh > 1) sh = 1;
      var ht = H[i] / R0; ht = ht > 1 ? 1 : ht;
      var cot = (1 + 0.2 * (2 * qfbm(u * 9 + seed, v * 9 + 3, 3) - 1)) * (1 + 0.03 * (2 * qfbm(u * 40 + 1, v * 40 + seed, 2) - 1));
      var lm = (0.12 * (1 - 0.7 * occ) + 0.92 * diff * (1 - 0.7 * sh) * (1 - 0.5 * occ) * (0.55 + 0.45 * Math.sqrt(ht))) * cot;
      lum[i] = lm < 0 ? 0 : lm > 1 ? 1 : lm;
      var fr = (halo[i] - 0.12) * 1.6; fr = (fr < 0 ? 0 : fr > 1 ? 1 : fr) * smooth(0.35, 0.65, qfbm(u * 9 + seed * 7, v * 9 + 1, 4)) * 0.65;
      al[i] = Math.max(C[i], fr) * (1 - smooth(0.82, 1, Math.sqrt(u * u + v * v)));
    }
    var sm = 0, sw = 0;                                            // how far from its centre the head reaches, as a fraction of its half-width
    for (y = 0; y < S; y++) for (x = 0; x < S; x++) { var aw = al[y * S + x]; sm += aw * (Math.pow((x + 0.5) / S * 2 - 1, 2) + Math.pow((y + 0.5) / S * 2 - 1, 2)); sw += aw; }
    var ext = 1.4 * Math.sqrt(sm / sw);
    for (i = 0; i < n; i++) { lg[i] = Math.pow(lum[i], 1.4); ll[i] = Math.pow(lum[i], 1.3); }
    return { lg: lg, ll: ll, al: al, sh: boxBlur(al, S, S, 3), ext: ext };
  }

  /* ---------- The bank: where each head stands ---------- */
  // Four sizes, from the broad dark heads that make the floor of the bank to the small ones that fray its edge.
  // [count, size from, size to, exposure at the bottom, exposure on top]; a head's centre keeps its edge inside the ring. The floor goes first;
  // the rest are laid by height, and the higher a head stands the more sun it takes.
  var STORM_BANK = [[40, 0.05, 0.07, 0.5, 0.7, 0.34, 0.365], [44, 0.08, 0.108, 0.5, 0.75], [26, 0.07, 0.095, 0.55, 1.25], [28, 0.045, 0.062, 0.6, 1.1], [16, 0.028, 0.04, 0.65, 0.95]];
  var STORM_SPEED = [0.8, 0.9, 1, 1.1, 1];
  function stormBuild() {
    var R = stormRand(11), out = [];
    if (!stormSprites) { stormSprites = []; for (var v = 0; v < STORM_NV; v++) stormSprites.push(stormSprite(v + 1)); }
    STORM_BANK.forEach(function (sp, c) {
      for (var i = 0; i < sp[0]; i++) {
        var z = R(), e = sp[3] + (sp[4] - sp[3]) * Math.pow(z, 1.6), lv = 0, k, s = sp[1] + (sp[2] - sp[1]) * R(), vv = Math.floor(R() * STORM_NV), ex = 0.8 * stormSprites[vv].ext * s;
        var a = (i + (R() - 0.5)) / sp[0] * TAU, bw = smooth(0.25, 0.75, qn(Math.cos(a) * 1.3 + 4, Math.sin(a) * 1.3 + 6)), lo = STORM_RMIN + ex + 0.03 * (1 - bw), hi = STORM_RMAX - ex - 0.03 * (1 - bw);
        if (sp[5]) { lo = sp[5]; hi = sp[6]; }
        for (k = 1; k < STORM_EXPO.length; k++) if (Math.abs(STORM_EXPO[k] - e) < Math.abs(STORM_EXPO[lv] - e)) lv = k;
        out.push({ c: c, z: c > 1 ? z : c - 2, a: a, r: lo + (hi - lo) * R(), s: s, v: vv, lv: lv, w: 0.35 + 0.45 * R(), p: R() * TAU, w2: 0.2 + 0.3 * R(), p2: R() * TAU });
      }
    });
    out.sort(function (a, b) { return a.z - b.z; });
    return out;
  }

  /* Palette: the cloud's ramp from the deep slate of the shaded creases to the pale grey of a lit top, the cold
     light a flash throws on a head, and the lightning's colours, under the grade in force. */
  var STORM_RAMP = [[0, [16, 20, 34]], [0.16, [38, 46, 70]], [0.34, [78, 90, 120]], [0.55, [132, 144, 170]], [0.75, [194, 202, 220]], [0.9, [234, 238, 246]], [1, [252, 252, 252]]];
  var STORM_LIT = [[0, [56, 68, 128]], [0.3, [120, 140, 212]], [0.55, [196, 212, 252]], [0.8, [244, 247, 255]], [1, [255, 255, 255]]];
  var stormPals = {};
  // A colour picker walks through many grades: keep the newest few of each cache.
  function stormKeep(map, limit) { var ks = Object.keys(map); if (ks.length >= limit) delete map[ks[0]]; }
  function stormLut(ramp) {
    var g = ramp.map(function (e) { return [e[0], tc(e[1])]; }), r = new Uint8Array(256 * 3), i, c;
    for (i = 0; i < 256; i++) { c = rampAt(g, i / 255); r[i * 3] = c[0]; r[i * 3 + 1] = c[1]; r[i * 3 + 2] = c[2]; }
    return r;
  }
  function stormPal() {
    var k = gradeId();
    if (!stormPals[k]) stormKeep(stormPals, 16);
    return stormPals[k] || (stormPals[k] = { cloud: stormLut(STORM_RAMP), lit: stormLut(STORM_LIT), shade: tc([8, 10, 20]), flash: tc([176, 198, 255]), glow: tc([120, 112, 255]), core: tc([240, 246, 255]) });
  }
  // The sprites of every head under the grade in force: each variant at each exposure, lit by a flash, and as a shade.
  function stormSet() {
    var k = gradeId(), st = stormSets[k];
    if (st) return st;
    stormKeep(stormSets, 4);
    var pal = stormPal(), S = STORM_SP, n = S * S, cloud = pal.cloud, lit = pal.lit;
    // one sprite canvas from a luminance (looked up in a palette), a scale on it and an alpha
    function paint(lm, pl, e, al, aMul) {
      var cv = document.createElement('canvas'), g, img, d, i, o, ix, v;
      cv.width = cv.height = S; g = cv.getContext('2d'); img = g.createImageData(S, S); d = img.data;
      for (i = 0; i < n; i++) {
        if (al[i] <= 0) continue;
        o = i * 4; v = lm ? lm[i] * e : 0; ix = (v <= 0 ? 0 : v >= 1 ? 255 : (v * 255) | 0) * 3;
        d[o] = pl[ix]; d[o + 1] = pl[ix + 1]; d[o + 2] = pl[ix + 2]; d[o + 3] = al[i] * aMul;
      }
      g.putImageData(img, 0, 0); return mips(cv);
    }
    // each sprite also at half and quarter size, so a head drawn small is not sampled from far above its size
    function mips(cv) {
      var out = [cv], w = cv.width, c, m;
      while (w > 32) { w >>= 1; c = document.createElement('canvas'); c.width = c.height = w; m = c.getContext('2d'); m.drawImage(out[out.length - 1], 0, 0, w, w); out.push(c); }
      return out;
    }
    var shade = new Uint8Array(768); shade[0] = pal.shade[0]; shade[1] = pal.shade[1]; shade[2] = pal.shade[2];
    st = stormSets[k] = { sp: [], lit: [], sh: [] };
    stormSprites.forEach(function (spr) {
      st.sp.push(STORM_EXPO.map(function (e) { return paint(spr.lg, cloud, e, spr.al, 255); }));
      st.lit.push(paint(spr.ll, lit, 1, spr.al, 255));
      st.sh.push(paint(null, shade, 0, spr.sh, 190));
    });
    return st;
  }
  // How much a flash lights a head: the light falls off with distance, and a head is wide, so it spreads with the head.
  function stormGain(x, y, rad, lights) {
    var g = 0, i;
    for (i = 0; i < lights.length; i++) {
      var L = lights[i], dx = L[0] - x, dy = L[1] - y, s2 = L[2] * L[2] + rad * rad * 0.12, d2 = dx * dx + dy * dy;
      if (d2 > 30 * s2) continue;
      g += L[3] * (stormExp(d2 / (2 * s2)) + 0.22 * stormExp(d2 / (2 * s2 * 3.24)));
    }
    g *= 0.7; return g > 0.92 ? 0.92 : g;
  }
  // The size the bank is painted at for a canvas this wide: the cost of a frame goes with its area.
  function stormSize(w) { return Math.max(256, Math.min(640, Math.ceil(w * 1.2 / 32) * 32)); }
  // The bank at time t, painted back to front into a square canvas the size of the token.
  function stormTexture(t, lights, N) {
    var tk = gradeId() + '|' + N, q = Math.round(t * 30), c = stormTex[tk];
    if (c && c.q === q) return c;
    if (!stormP) stormP = stormBuild();
    if (!c) { stormKeep(stormTex, 8); c = stormTex[tk] = { cv: document.createElement('canvas') }; c.cv.width = c.cv.height = N; }
    var set = stormSet(), g = c.cv.getContext('2d'), sx = -STORM_SUN[0], sy = -STORM_SUN[1], hy = Math.hypot(sx, sy);
    g.clearRect(0, 0, N, N);
    stormP.forEach(function (p) {
      var a = p.a + STORM_ROT * STORM_SPEED[p.c] * t, r = p.r + 0.004 * Math.sin(p.w2 * t + p.p2), px = Math.cos(a) * r, py = Math.sin(a) * r;
      var d = 2 * p.s * N * (1 + 0.09 * Math.sin(p.w * t + p.p)), x0 = N / 2 + px * N - d / 2, y0 = N / 2 + py * N - d / 2, gn = lights.length ? stormGain(px, py, p.s, lights) : 0, m = d > 100 ? 0 : d > 50 ? 1 : 2;
      if (p.c > 1) { g.globalAlpha = 1; g.drawImage(set.sh[p.v][m], x0 + sx / hy * d * 0.16, y0 + sy / hy * d * 0.16, d, d); }
      g.globalAlpha = 1; g.drawImage(set.sp[p.v][p.lv][m], x0, y0, d, d);
      if (gn > 0.02) { g.globalAlpha = gn; g.drawImage(set.lit[p.v][m], x0, y0, d, d); }
    });
    g.globalAlpha = 1; c.q = q;
    return c;
  }

  /* ---------- Lightning: when, where and what it looks like ---------- */
  // Most flashes happen inside the cloud and are seen only as the billows lighting up; now and then a bolt crawls
  // across the top or strikes out over the map. The still frame (t = 2.5) lands on the return stroke of a strike.
  function stormEvent(j) {
    var r1 = hash(j, 31), r2 = hash(j, 32), te = 0.45 + j * 1.7 + r1 * 0.8;
    var kind = r2 < 0.34 ? 'flash' : r2 < 0.46 ? 'sheet' : r2 < 0.66 ? 'crawl' : r2 < 0.8 ? 'arc' : 'fork', a = hash(j, 33) * TAU;
    if (j === 1) { te = 2.5; kind = 'arc'; a = -1.3; }
    return { j: j, q0: Math.round(te * 30), kind: kind, a: a, nb: 2 + Math.floor(hash(j, 34) * 3.99), dec: 3.5 + 3 * hash(j, 35) };
  }
  var STORM_STROKE = [1, 0.5, 0.92, 0.42, 0.7, 0.3];
  function stormTri(f, c, w) { return Math.max(0, 1 - Math.abs(f - c) / w); }
  // How bright the cloud glow and the bolt are f frames after the return stroke.
  function stormFlash(ev, f) {
    var glow = 0, bolt = 0, after = 0, reveal = 1, h = hash(ev.j * 7 + f, 41);
    if (f < -2 || f > 24) return null;
    if (ev.kind === 'sheet') {
      glow = (stormTri(f, 1, 2.5) + 0.65 * stormTri(f, 5, 2) + 0.4 * stormTri(f, 9, 3)) * (0.75 + 0.25 * h) * 0.8 + 0.12 * Math.exp(-Math.max(0, f) / 6);
    } else if (ev.kind === 'flash') {
      glow = (stormTri(f, 0, 1.6) + 0.75 * stormTri(f, 3.5, 1.6) + 0.5 * stormTri(f, 7, 2) + 0.22 * stormTri(f, 11, 3)) * (0.8 + 0.2 * h);
    } else {
      if (f < 0) { glow = 0.25 + 0.15 * (f + 2); reveal = 0.35 + 0.3 * (f + 2); bolt = 0.25; }
      else {
        glow = Math.exp(-f / ev.dec) * (0.72 + 0.28 * hash(ev.j * 5 + f, 42)) + 0.35 * stormTri(f, 3, 2);
        if (f < ev.nb) bolt = STORM_STROKE[f] * (0.85 + 0.15 * hash(ev.j * 9 + f, 43));
        else if (f < ev.nb + 4) after = 0.22 * Math.pow(0.5, f - ev.nb);
      }
    }
    return { glow: glow, bolt: bolt, after: after, reveal: reveal };
  }
  function stormActive(q) {
    var out = [], j0 = Math.floor((q / 30 - 0.45) / 1.7);
    for (var j = Math.max(0, j0 - 2); j <= j0 + 2; j++) {
      var ev = stormEvent(j), lv = stormFlash(ev, q - ev.q0);
      if (lv) out.push({ ev: ev, f: q - ev.q0, lv: lv });
    }
    return out;
  }

  /* ---------- Bolt geometry, in token units from the centre ---------- */
  // A channel from a to b: split at irregular points and pushed sideways by an uneven amount, with now and then
  // a hard kink, so the runs between turns vary as they do in a real discharge.
  function stormChannel(R, a, b, depth, amp, out) {
    var dx = b[0] - a[0], dy = b[1] - a[1], len = Math.hypot(dx, dy);
    if (depth === 0 || len < 0.004) { out.push(b); return; }
    var f = 0.27 + 0.46 * R(), off = (R() + R() + R() - 1.5) * amp * len * (R() < 0.14 ? 2.3 : 1);
    var m = [a[0] + dx * f - dy / len * off, a[1] + dy * f + dx / len * off];
    stormChannel(R, a, m, depth - 1, amp, out); stormChannel(R, m, b, depth - 1, amp, out);
  }
  function stormPoly(pts, w, s0, s1, life) { // params s run along the discharge, so a bolt can grow from its start
    var cum = [0], i, tot = 0;
    for (i = 1; i < pts.length; i++) { tot += Math.hypot(pts[i][0] - pts[i - 1][0], pts[i][1] - pts[i - 1][1]); cum.push(tot); }
    return { p: pts, s: cum.map(function (c) { return s0 + (s1 - s0) * (tot ? c / tot : 0); }), w: w, life: life, len: tot };
  }
  function stormBranches(R, polys, parent, depth, count, spread, reach) {
    var pts = parent.p, n = pts.length, made = 0;
    for (var tries = 0; tries < count * 5 && made < count; tries++) {
      var j = 2 + Math.floor(R() * (n - 4)); if (j < 2 || j >= n - 2) continue;
      var dx = pts[j + 1][0] - pts[j - 1][0], dy = pts[j + 1][1] - pts[j - 1][1], dl = Math.hypot(dx, dy) || 1;
      var ang = (R() < 0.5 ? -1 : 1) * (0.3 + spread * R()), ca = Math.cos(ang), sa = Math.sin(ang);
      var ux = (dx * ca - dy * sa) / dl, uy = (dx * sa + dy * ca) / dl, len = reach * (0.35 + 0.65 * R()) * (depth ? 1 : 0.5);
      var out = [pts[j]]; stormChannel(R, pts[j], [pts[j][0] + ux * len, pts[j][1] + uy * len], depth ? 4 : 3, 0.3, out);
      var s0 = Math.min(0.9, parent.s[j] + 0.05), br = stormPoly(out, parent.w * (depth ? 0.55 : 0.5), s0, Math.min(1, s0 + 0.28), 1 + Math.floor(R() * 4.5));
      polys.push(br); made++;
      if (depth) stormBranches(R, polys, br, 0, 1 + Math.floor(R() * 2), 0.9, len);
    }
  }
  function stormPolar(a, r) { return [Math.cos(a) * r, Math.sin(a) * r]; }
  var stormBolts = {};
  // The channel of a strike or crawler, and the points along it that light the cloud (x, y, spread, strength).
  function stormBolt(ev) {
    if (stormBolts[ev.j]) return stormBolts[ev.j];
    var R = stormRand(ev.j * 13 + 5), polys = [], a = ev.a, src = [], main, k;
    if (ev.kind === 'fork') {
      var a1 = a + (R() < 0.5 ? 1 : -1) * (0.3 + 0.25 * R()), p0 = stormPolar(a, 0.345 + 0.02 * R()), pts = [p0];
      stormChannel(R, p0, stormPolar(a1, 0.482), 7, 0.22, pts);
      main = stormPoly(pts, 1, 0, 1, 99); polys.push(main);
      stormBranches(R, polys, main, 1, 6 + Math.floor(R() * 3), 0.9, 0.11);
      for (k = 0; k < pts.length; k += 5) src.push([pts[k][0], pts[k][1], 0.05, 0.9 - 0.5 * k / pts.length]);
    } else if (ev.kind === 'flash' || ev.kind === 'sheet') {
      return stormBolts[ev.j] = { polys: [], src: [] };
    } else {
      var crawl = ev.kind === 'crawl', span = (crawl ? 1.0 + 0.7 * R() : 1.1 + 0.5 * R()) * (R() < 0.5 ? 1 : -1), nw = crawl ? 8 : 7, wp = [stormPolar(a, 0.4)], pts2 = [wp[0]];
      for (k = 1; k <= nw; k++) wp.push(stormPolar(a + span * k / nw, 0.405 + (R() - 0.4) * 0.07));
      for (k = 1; k < wp.length; k++) stormChannel(R, wp[k - 1], wp[k], crawl ? 3 : 4, 0.28, pts2);
      main = stormPoly(pts2, crawl ? 0.7 : 1, 0, 1, 99); polys.push(main);
      stormBranches(R, polys, main, 1, crawl ? 8 : 7 + Math.floor(R() * 3), 1.0, crawl ? 0.05 : 0.09);
      for (k = 0; k < pts2.length; k += 6) src.push([pts2[k][0], pts2[k][1], 0.05, 0.85]);
    }
    return stormBolts[ev.j] = { polys: polys, src: src };
  }
  // The points that light the cloud this frame: x, y, spread, strength, height of the source above the cloud floor.
  function stormLights(act) {
    var out = [];
    act.forEach(function (e) {
      var s = e.lv.glow; if (s <= 0.01) return;
      var a = e.ev.a, R = stormRand(e.ev.j * 29 + 3), q, rc = 0.405;
      if (e.ev.kind === 'sheet') {
        for (q = 0; q < 3; q++) out.push([Math.cos(a + q * 0.55) * rc, Math.sin(a + q * 0.55) * rc, 0.1 - 0.012 * q, s * (1.7 - 0.4 * q), 0.04]);
      } else if (e.ev.kind === 'flash') {
        var ng = 1 + (R() < 0.5 ? 1 : 0);
        for (q = 0; q < ng; q++) { var aa = a + q * (0.35 + 0.3 * R()), rr = rc + (R() - 0.5) * 0.05; out.push([Math.cos(aa) * rr, Math.sin(aa) * rr, 0.07 + 0.02 * R(), s * (2.4 - 0.7 * q), 0.035]); }
      } else stormBolt(e.ev).src.forEach(function (p) { out.push([p[0], p[1], p[2], s * p[3] * 1.6, 0.04]); });
    });
    var gs = 0; out.forEach(function (l) { gs += l[3]; }); out.gsum = Math.min(1.2, gs);
    return out;
  }

  /* ---------- Drawing ---------- */
  var stormHazeCv = document.createElement('canvas');
  function stormHaze(tex) { // the cloud's shade on the map and the picture: a blur made by shrinking it
    var cv = stormHazeCv, g = cv.getContext('2d'); cv.width = cv.height = 40;
    g.imageSmoothingQuality = 'high'; g.drawImage(tex, 0, 0, 40, 40);
    g.globalCompositeOperation = 'source-in'; g.fillStyle = 'rgb(2,3,8)'; g.fillRect(0, 0, 40, 40);
    return cv;
  }
  // One polyline of a bolt as stacked strokes in chunks: a faint wide glow, a blue halo, a white-blue shell and a
  // white-hot core, each chunk fading toward the canvas edge and the picture.
  function stormStroke(ctx, poly, rev, lv, S, pal, taperEnd, f) {
    var p = poly.p, n = p.length, last = n - 1, i, ch = 7, c0, nc = Math.ceil((n - 1) / ch);
    while (last > 0 && poly.s[last] > rev) last--;
    for (c0 = 0; c0 < last; c0 += ch) {
      var c1 = Math.min(last, c0 + ch), ci = c0 / ch, mid = p[(c0 + c1) >> 1], taper = 1 - taperEnd * (ci / nc), rm = Math.hypot(mid[0], mid[1]);
      var edge = (1 - smooth(0.455, 0.492, rm)) * smooth(0.3, 0.345, rm); if (edge <= 0.01) continue;
      var w = poly.w * taper, a = lv * taper * edge * (0.82 + 0.18 * hash(ci + poly.len * 97, f));
      ctx.beginPath(); ctx.moveTo(p[c0][0] * S + S / 2, p[c0][1] * S + S / 2);
      for (i = c0 + 1; i <= c1; i++) ctx.lineTo(p[i][0] * S + S / 2, p[i][1] * S + S / 2);
      ctx.globalAlpha = 0.08 * a; ctx.strokeStyle = rgba(pal.glow, 1); ctx.lineWidth = Math.max(4, w * S * 0.022); ctx.stroke();
      ctx.globalAlpha = 0.28 * a; ctx.strokeStyle = rgba(pal.flash, 1); ctx.lineWidth = Math.max(2, w * S * 0.0075); ctx.stroke();
      ctx.globalAlpha = 0.7 * a; ctx.strokeStyle = rgba(mix3(pal.flash, pal.core, 0.6), 1); ctx.lineWidth = Math.max(1.3, w * S * 0.0036); ctx.stroke();
      ctx.globalAlpha = Math.min(1, a * 1.15); ctx.strokeStyle = rgba(pal.core, 1); ctx.lineWidth = Math.max(0.8, w * S * 0.0017); ctx.stroke();
    }
  }
  function stormBoltDraw(ctx, e, S, pal) {
    var lv = e.lv, b = stormBolt(e.ev), lvl = lv.bolt || lv.after, f = e.f, strong = lv.bolt > 0;
    if (lvl <= 0.01 || !b.polys.length) return;
    ctx.save(); ctx.beginPath(); ctx.arc(S / 2, S / 2, S * 0.487, 0, TAU); ctx.clip(); // a branch's tip may run past the cloud: nothing reaches the canvas edge
    ctx.globalCompositeOperation = 'lighter'; ctx.lineCap = 'round'; ctx.lineJoin = 'round';
    b.polys.forEach(function (poly, pi) {
      if (f >= 0 && !(poly.life > f) && strong) return;                          // forks drop out as the discharge ebbs
      if (!strong && pi > 0 && poly.len < 0.05) return;
      stormStroke(ctx, poly, lv.reveal, lvl, S, pal, pi ? 0.65 : 0.3, f);
    });
    ctx.restore();
  }
  function stormFront(ctx, G, t) {
    var W = G.outW, H = G.outH, q = Math.round(t * 30), pal = stormPal(), act = stormActive(q), lights = stormLights(act), i;
    var tex = stormTexture(t, lights, stormSize(W)).cv;
    ctx.save(); ctx.imageSmoothingQuality = 'high';
    ctx.globalAlpha = 0.5; ctx.drawImage(stormHaze(tex), W * 0.006, H * 0.012, W, H);   // the shade it throws, away from the sun
    ctx.globalAlpha = 1; ctx.drawImage(tex, 0, 0, W, H);
    ctx.restore(); ctx.save(); ctx.globalCompositeOperation = 'lighter';           // the flash lights the picture's edge and the map
    for (i = 0; i < lights.length; i++) {
      var L = lights[i], gx = W / 2 + L[0] * W, gy = H / 2 + L[1] * H, rad = Math.min(W * 0.26, W * 0.495 - Math.max(Math.abs(L[0]), Math.abs(L[1])) * W); // the glow stays inside the canvas
      if (rad < W * 0.04) continue;
      var gr = ctx.createRadialGradient(gx, gy, 0, gx, gy, rad);
      var ga = Math.min(0.16, 0.1 * L[3]); // eased by hand: a straight ramp to nothing shows as a disc
      gr.addColorStop(0, rgba(pal.flash, ga)); gr.addColorStop(0.25, rgba(pal.flash, ga * 0.6)); gr.addColorStop(0.55, rgba(pal.flash, ga * 0.2)); gr.addColorStop(0.8, rgba(pal.flash, ga * 0.04)); gr.addColorStop(1, rgba(pal.flash, 0));
      ctx.fillStyle = gr; ctx.fillRect(gx - rad, gy - rad, rad * 2, rad * 2);
    }
    ctx.restore();
    act.forEach(function (e) { stormBoltDraw(ctx, e, W, pal); });
  }
  EXTRA.push({ id: 'storm', name: 'Thunderstorm', blurb: 'A ring of towering storm cloud rolling slowly round the token, lit from inside by lightning, with the odd bolt striking out over the map.',
    key: [120, 150, 220], luma: 0.4, spread: 0.5,
    spec: { anim: true, roundOnly: true, scale: STORM_SC, rim: 0.1, feather: 0.035, front: stormFront } });
  /* ---------- Watercolour: a loose ring of ultramarine washed straight onto the map ----------
     There is no paper: the ring is three brush strokes of one pigment family laid on whatever lies beneath the token,
     each stroke wet when it went down and each starting over the dry end of the last. The pigment is worked out as
     density, the way it lies on a sheet: it pools at the edge where a wash dried (edge darkening), sinks into the valleys
     of an unseen tooth (granulation), thins where the wash ran, is pushed back by water dropped into it (the cauliflower
     bloom), and skips where the brush ran dry. Light passes through that density, so overlaps glaze darker and bluer.
     With no sheet to multiply onto, the density is then turned into the paint's own colour and opacity: thin wash lets
     the map show through, while pooled edges and overlaps are dense and deep. The strokes keep their inner edge at the
     picture and let the width swell outward, so the ring hugs the picture and is loose only on the outside. */
  var WATERCOLOR_NA = 1024;
  // One entry per wash. turn: where the stroke starts (a turn is 0 at nine o'clock, running clockwise) and span how far it
  // goes; ri, hw: the radius of its inner edge (kept at the picture, since the painter worked up to it) and its half width,
  // as fractions of the token size; d: pigment load; k: how strongly each of red, green and blue is absorbed (ultramarine
  // eats red hardest, the violet-leaning mix a little more green); hardIn, hardOut: how crisp each edge is; wander: how far
  // the inner edge strays from a true circle; slant: how much earlier the outer edge of the brush touches down and lifts.
  var WATERCOLOR_WASHES = [
    { turn: 0.58, span: 0.80, ri: 0.358, hw: 0.056, d: 0.58, k: [1.75, 1.32, 0.55], hardIn: 0.9, hardOut: 0.7, wander: 0.006, slant: 0.003, seed: 1, gk: 0.9 },
    { turn: 0.27, span: 0.52, ri: 0.360, hw: 0.058, d: 0.60, k: [1.55, 1.62, 0.55], hardIn: 0.65, hardOut: 0.9, wander: 0.006, slant: 0.004, seed: 2, gk: 1.0 },
    { turn: 0.02, span: 0.42, ri: 0.362, hw: 0.040, d: 0.64, k: [1.80, 1.38, 0.62], hardIn: 0.9, hardOut: 0.9, wander: 0.006, slant: 0.003, seed: 3, gk: 1.0 }
  ];
  // Blooms: wash index, where (turn, offset from the wash centre line in token units), size.
  var WATERCOLOR_BLOOMS = [
    { w: 0, a: 0.72, o: 0.004, R: 0.044, s: 1 }, { w: 0, a: 0.10, o: -0.002, R: 0.046, s: 2 },
    { w: 1, a: 0.44, o: 0.002, R: 0.034, s: 3 }, { w: 1, a: 0.60, o: 0.006, R: 0.022, s: 4 },
    { w: 2, a: 0.16, o: 0.0, R: 0.024, s: 5 }, { w: 0, a: 0.92, o: 0.0, R: 0.03, s: 6 }
  ];

  // The unseen tooth the pigment settles into: rounded grains of two sizes over a slow mottle, and two soft speckle fields
  // (single grains, and clumps of them) made by blurring noise, so nothing is finer than a few pixels.
  function watercolorTooth(w, h) {
    var S = Math.min(w, h), cx = w / 2, cy = h / 2, n = w * h, H = new Float32Array(n), x, y, i;
    for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
      var X = (x + 0.5 - cx) / S, Y = (y + 0.5 - cy) / S, r = Math.hypot(X, Y);
      if (r < 0.3 || r > 0.505) continue;
      var v1 = voronoi(X * 150 + 7, Y * 150), h1 = (1 - smooth(0.05, 0.85, v1.f1)) * (0.3 + 0.7 * v1.id);
      var v2 = voronoi(X * 68 + 2, Y * 68 + 5), h2 = (1 - smooth(0.05, 0.9, v2.f1)) * (0.5 + 0.5 * v2.id);
      H[y * w + x] = 0.3 * h1 + 0.2 * h2 + 0.25 * fbm(X * 60 + 4, Y * 60 + 9, 3) + 0.2 * fbm(X * 20 + 8, Y * 20, 3);
    }
    var seed = 12345, tmp = new Float32Array(n), out = [];
    [Math.max(1, Math.round(S * 0.0013)), Math.max(2, Math.round(S * 0.0042))].forEach(function (rad) {
      for (i = 0; i < n; i++) { seed = (seed * 16807) % 2147483647; tmp[i] = seed / 2147483647; }
      var B = boxBlur(tmp, w, h, rad), sum = 0, sq = 0;
      for (i = 0; i < n; i++) sum += B[i];
      var mean = sum / n;
      for (i = 0; i < n; i++) sq += (B[i] - mean) * (B[i] - mean);
      var sd = Math.sqrt(sq / n) || 1;
      for (i = 0; i < n; i++) B[i] = (B[i] - mean) / sd;
      out.push(B);
    });
    return { H: H, g1: out[0], g2: out[1] };
  }

  // Per-angle tables for one wash: where its centre line runs, how wide the brush was, how crisp each edge, how much
  // pigment was left, and how close it is to touching down or running dry.
  function watercolorTable(ws, NA) {
    var T = { rc: new Float32Array(NA), hw: new Float32Array(NA), hin: new Float32Array(NA), hout: new Float32Array(NA), dn: new Float32Array(NA), dry: new Float32Array(NA), lead: new Float32Array(NA) };
    var circ = TAU * (ws.ri + ws.hw), full = ws.span >= 1, sd = ws.seed * 5.3;
    for (var i = 0; i < NA; i++) {
      var a = i / NA, s = fract(a - ws.turn), cap = 1, dry = 0;
      if (!full) {
        if (s > ws.span) continue;
        var u0 = s * circ / ws.hw, Le = (ws.span - s) * circ;
        cap = (u0 < 1 ? Math.sqrt(1 - (1 - u0) * (1 - u0)) : 1) * (0.5 + 0.5 * Math.pow(smooth(0, 1, Le / 0.14), 0.7));
        dry = 1 - smooth(0.0, 0.22, Le);
      }
      var n1 = pfbm(a * 8 + sd, sd * 0.3, 8, 3), n2 = pfbm(a * 4 + sd + 9, sd * 0.2 + 1, 4, 2), n3 = pfbm(a * 8 + sd + 21, 3.7, 8, 2), n4 = pfbm(a * 8 + sd + 40, 6.1, 8, 2);
      // the brush pressed harder in places, so the outer edge swells; the soft limit keeps the paint inside the token
      var hwR = ws.hw * (0.3 + 1.7 * clamp((n2 - 0.5) * 1.4 + 0.5, 0.05, 1)) * (1 + 0.2 * Math.cos((a - 0.1) * TAU));
      var hwF = hwR / Math.pow(1 + Math.pow(hwR / 0.06, 4), 0.25);
      T.rc[i] = ws.ri + ws.wander * (n1 - 0.5) * 2 + hwF;                 // the inner edge stays at the picture; the width shows outside
      T.hw[i] = hwF * cap;
      T.hin[i] = clamp(ws.hardIn + (n3 - 0.5) * 0.9, 0.05, 1);
      T.hout[i] = clamp(ws.hardOut + (n4 - 0.5) * 0.9, 0.05, 1);
      var lead = s < 0.05 && !full ? 1 - s / 0.05 : 0, sn = full ? 0.4 : s / ws.span;
      T.dn[i] = (1.12 - 0.4 * smooth(0, 1, sn)) * (1 + 0.7 * lead * lead);   // the brush empties as it goes, and was fullest where it first touched down
      T.dry[i] = dry; T.lead[i] = lead;
    }
    return T;
  }

  // The pigment density of every pixel, one value per colour channel (how much of each is absorbed).
  function watercolorDensity(w, h) {
    var S = Math.min(w, h), cx = w / 2, cy = h / 2, tooth = watercolorTooth(w, h), H = tooth.H, G1 = tooth.g1, G2 = tooth.g2;
    var NA = WATERCOLOR_NA, MK = NA - 1, n = w * h;
    var DR = new Float32Array(n), DG = new Float32Array(n), DB = new Float32Array(n), x, y, j, b;
    var tabs = WATERCOLOR_WASHES.map(function (ws) { return watercolorTable(ws, NA); });
    var blooms = WATERCOLOR_WASHES.map(function (ws, wi) {
      return WATERCOLOR_BLOOMS.filter(function (bl0) { return bl0.w === wi; }).map(function (bl0) {
        var a = bl0.a * TAU - Math.PI, rr = ws.ri + ws.hw + bl0.o;
        return { x: Math.cos(a) * rr, y: Math.sin(a) * rr, R: bl0.R, s: bl0.s * 7.7 };
      });
    });
    var A = [0, 0, 0];
    for (y = 0; y < h; y++) for (x = 0; x < w; x++) {
      var X = (x + 0.5 - cx) / S, Y = (y + 0.5 - cy) / S, r = Math.hypot(X, Y), o = y * w + x;
      if (r < 0.33 || r > 0.5) continue;
      var a = Math.atan2(Y, X) / TAU + 0.5, th = H[o], g1 = G1[o], g2 = G2[o];
      A[0] = A[1] = A[2] = 0;
      for (j = 0; j < tabs.length; j++) {
        var T = tabs[j], ws = WATERCOLOR_WASHES[j];
        // the brush was tilted, so its outer edge touches down and lifts a little before its inner edge
        var wa = fbm(X * 8 + ws.seed * 7.1, Y * 8 + 2, 3) - 0.5, ai = (a - ws.slant * clamp((r - ws.ri) / ws.hw - 1, -1.2, 1.2) + 0.03 * wa) * NA, i0 = Math.floor(ai) & MK, i1 = (i0 + 1) & MK, fr = ai - Math.floor(ai);
        var hw = T.hw[i0] + (T.hw[i1] - T.hw[i0]) * fr;
        if (hw < 0.0012) continue;
        var rc = T.rc[i0] + (T.rc[i1] - T.rc[i0]) * fr, dr = r - rc, adr = Math.abs(dr);
        if (adr > hw + 0.045) continue;
        var inner = dr < 0, hard = inner ? T.hin[i0] + (T.hin[i1] - T.hin[i0]) * fr : T.hout[i0] + (T.hout[i1] - T.hout[i0]) * fr;
        var lead = T.lead[i0]; hard *= 1 - 0.75 * lead;               // a stroke's first touch is wet and soft-edged
        // the edge of the wash: a coastline at several scales, the tooth nibbling at it, and where the edge was wet the
        // water ran out along the fibres in short fingers
        var o1 = ws.seed * 13.1, nE = fbm(X * 11 + o1, Y * 11 + o1 * 0.7, 4) - 0.5, nF = vnoise(X * 55 + o1, Y * 55) - 0.5;
        var rho = hw - adr + (inner ? 0.012 : 0.026) * nE + 0.004 * nF + 0.0016 * (th - 0.45);
        rho += (1 - hard) * 0.016 * (pvn(a * 220 + o1, r * 90, 256) - 0.5) * (0.5 + pvn(a * 40 + o1, r * 30, 64));
        var dry = T.dry[i0], rag = lead + dry;
        if (rag > 0.01) rho += rag * 0.03 * (fbm(X * 22 + o1, Y * 22 + 3, 3) - 0.5) * 2 + lead * 0.01 * (pvn(a * 96 + o1, r * 40, 128) - 0.5) * 2;     // touch-down and lift-off are ragged
        var sft = 0.0012 + (1 - hard) * 0.005 + lead * 0.006, m = smooth(-sft, sft, rho);
        if (m <= 0) continue;
        var ins = rho > 0 ? rho : 0, dd = ws.d * T.dn[i0];
        var nB = fbm(X * 6 + o1, Y * 6, 3), tn = fbm(X * 3.2 + o1 * 1.3, Y * 3.2 + 5, 3), flow = pvn(a * 14 + o1, r * 26, 16);
        var streak = pvn(a * 128 + o1, r * 160, 128) * 0.6 + pvn(a * 40 + o1, r * 110, 64) * 0.4;
        var base = dd * (0.3 + 1.0 * (nB - 0.25)) * (1 - 0.25 * (1 - (adr / hw) * (adr / hw))) * (0.82 + 0.36 * streak) * (0.7 + 0.7 * flow);
        base *= 1 + 0.4 * smooth(0.5, 0.54, tn);                          // where the wash dried in stages
        var grav = 0.75 + 0.5 * smooth(-0.3, 0.3, Y);                       // pigment runs down and pools along the lower edges
        // the pool at the edge comes and goes along it, and the inner edge, against the picture, holds a little more
        var eM = 0.35 + 1.25 * pvn(a * 26 + o1 * 3 + (inner ? 50 : 0), 3.3, 32), pool = (inner ? 1.15 : 1) * (1 - 0.6 * lead) * eM;
        var edge = hard * hard * Math.exp(-ins / 0.0042) * 1.5 * grav * pool + 0.3 * Math.exp(-ins / 0.016) + 0.45 * Math.exp(-Math.abs(tn - 0.515) / 0.005) * hard;
        var dens = m * (base + dd * edge);
        if (dry > 0.01) {                   // the dry brush: bristle streaks along the stroke, the paint kept only on the high points of the tooth
          var bn = pvn(a * 32 + o1, r * 200, 32) * 0.55 + pvn(a * 64 + o1 * 2, r * 120, 64) * 0.2 + th * 0.35, thr = 0.78 * dry * dry + 0.1 * dry;
          dens *= smooth(thr - 0.1, thr + 0.1, bn);
        }
        // granulation: pigment settles into the valleys of the tooth, and single grains show, more in some places than others
        var gl = 0.35 + 1.3 * fbm(X * 9 + 3, Y * 9 + o1, 2);
        dens *= clamp(1 + ws.gk * gl * (0.45 * (0.46 - th) + 0.16 * g1 + 0.07 * g2), 0.5, 1.8);
        var bs = blooms[j];
        for (b = 0; b < bs.length; b++) {
          var bb = bs[b], dx = X - bb.x, dy = Y - bb.y, rr2 = dx * dx + dy * dy;
          if (rr2 > bb.R * bb.R * 3.2) continue;
          var qx = dx / bb.R, qy = dy / bb.R, wq = vnoise(qx * 1.3 + bb.s, qy * 1.3 + 2) - 0.5, wr = vnoise(qx * 1.3 + 7, qy * 1.3 + bb.s) - 0.5;
          qx += wq * 0.9; qy += wr * 0.9;     // so the bloom is never a circle
          var bf = 1 - Math.sqrt(qx * qx + qy * qy) + 0.3 * Math.abs(2 * vnoise(qx * 1.9 + bb.s, qy * 1.9) - 1) + 0.18 * Math.abs(2 * vnoise(qx * 4.4 + bb.s, qy * 4.4 + 3) - 1)
            + 0.09 * Math.abs(2 * vnoise(qx * 9.7 + bb.s, qy * 9.7 + 8) - 1) + 0.04 * Math.abs(2 * vnoise(qx * 21 + bb.s, qy * 21 + 1) - 1) - 0.3;
          var beta = smooth(0, 0.07, bf), rim = Math.exp(-Math.abs(bf * bb.R + 0.0012) / 0.0034), halo = Math.exp(-Math.max(0, -bf * bb.R) / 0.01);
          dens = dens * (1 - 0.52 * beta) + dd * (1.0 * rim + 0.35 * halo * (1 - beta)) * m;
        }
        // separation: where the pigment is thick the violet granules lead, where thin the blue
        var nC = fbm(X * 3.5 + o1 * 0.3, Y * 3.5, 2) - 0.5;
        A[0] += ws.k[0] * (1 - 0.12 * nC) * dens; A[1] += ws.k[1] * (1 + 0.35 * nC) * dens; A[2] += ws.k[2] * dens;
      }
      DR[o] = A[0]; DG[o] = A[1]; DB[o] = A[2];
    }
    // Splatter, flicked off the brush: a big drop or two with a spray of small ones, each a little long in the direction it
    // flew, with a darker rim that shows only on the larger drops.
    var k0 = WATERCOLOR_WASHES[0].k;
    function drop(px, py, rad, load, ex, dx, dy) {
      var x0 = Math.max(0, Math.floor((cx + px * S) - rad * S * 1.8 - 2)), x1 = Math.min(w - 1, Math.ceil((cx + px * S) + rad * S * 1.8 + 2));
      var y0 = Math.max(0, Math.floor((cy + py * S) - rad * S * 1.8 - 2)), y1 = Math.min(h - 1, Math.ceil((cy + py * S) + rad * S * 1.8 + 2));
      var rimK = 0.15 + rad * 55;
      for (var yy = y0; yy <= y1; yy++) for (var xx = x0; xx <= x1; xx++) {
        var X = (xx + 0.5 - cx) / S - px, Y = (yy + 0.5 - cy) / S - py;
        var al = X * dx + Y * dy, ac = Y * dx - X * dy;
        var dist = Math.hypot(al / (1 + ex), ac * (1 + ex)), wob = 1 + 0.2 * (vnoise(X * 120 + px * 99, Y * 120) - 0.5);
        var e = rad * wob - dist;
        if (e < -0.0012) continue;
        var cov = smooth(-0.0009, 0.0009, e), dens = cov * load * (0.7 + rimK * Math.exp(-Math.max(e, 0) / (0.25 * rad + 0.0006)));
        dens *= 1 + 0.5 * (0.5 - H[yy * w + xx]);
        var oo = yy * w + xx;
        DR[oo] += k0[0] * dens; DG[oo] += k0[1] * dens; DB[oo] += k0[2] * dens;
      }
    }
    for (var c = 0; c < 5; c++) {
      var ca = hash(c, 41) * TAU, cr = 0.452 + 0.03 * hash(c, 42), fa = ca + 1.2 * (hash(c, 43) - 0.5), dirx = Math.cos(fa), diry = Math.sin(fa);
      var bx = Math.cos(ca) * cr, by = Math.sin(ca) * cr, big = 0.0055 + 0.0055 * hash(c, 44);
      drop(bx, by, big, 0.65, 0.12, dirx, diry);
      for (var q = 0; q < 9; q++) {
        var sdist = (0.012 + 0.075 * hash(c * 9 + q, 45)) * (hash(c * 9 + q, 46) > 0.4 ? 1 : -1), side = (hash(c * 9 + q, 47) - 0.5) * 0.05;
        var px2 = bx + dirx * sdist - diry * side, py2 = by + diry * sdist + dirx * side, rd = 0.0011 + 0.0042 * Math.pow(hash(c * 9 + q, 48), 2.2), pr = Math.hypot(px2, py2);
        if (pr - rd < 0.388 || pr + rd > 0.494) continue;
        drop(px2, py2, rd, 0.6, 0.35 * hash(c * 9 + q, 49), dirx, diry);
      }
    }
    return [DR, DG, DB];
  }

  // Density to paint. Over white, a wash shows exp(-density) in each channel; the same look is written here as a colour
  // plus an opacity. LAM makes the opacity rise faster than the density, which lightens the colour needed to match
  // over white, and LIFT lightens thin washes further, so thin paint reads as a pale tint on a dark map and the
  // densest paint keeps a deep ultramarine floor rather than going black. STRENGTH is how much pigment the washes carry.
  var WATERCOLOR_LAM = 1.6, WATERCOLOR_LIFT = 0.5, WATERCOLOR_STRENGTH = 0.82;
  function watercolorPaint(w, h) {
    var D = watercolorDensity(w, h), DR = D[0], DG = D[1], DB = D[2], n = w * h;
    var cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d'), img = g.createImageData(w, h), d = img.data;
    for (var i = 0; i < n; i++) {
      var ar = DR[i] * WATERCOLOR_STRENGTH, ag = DG[i] * WATERCOLOR_STRENGTH, ab = DB[i] * WATERCOLOR_STRENGTH, am = Math.max(ar, ag, ab);
      if (am < 0.006) continue;
      var al = 1 - Math.exp(-WATERCOLOR_LAM * am), o = i * 4, inv = 255 / al, k = 1 - al, f = WATERCOLOR_LIFT * k * k;
      var cr = clamp((Math.exp(-ar) - k) * inv, 16, 255), cg = clamp((Math.exp(-ag) - k) * inv, 28, 255), cb = clamp((Math.exp(-ab) - k) * inv, 100, 255);
      d[o] = cr + (255 - cr) * f; d[o + 1] = cg + (255 - cg) * f; d[o + 2] = cb + (255 - cb) * f;
      d[o + 3] = 255 * al;
    }
    g.putImageData(img, 0, 0);
    return cv;
  }
  // Pale paint on a pale map has nothing to read against, so under a light recolour the wash leaves a faint damp shadow,
  // kept out of the recolouring.
  function watercolorShade(w, h) {
    var S = Math.min(w, h), cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    var g = cv.getContext('2d');
    g.filter = 'blur(' + Math.max(1, S * 0.006) + 'px)';
    g.drawImage(masterLayer('watercolor-paint', w, h, watercolorPaint), S * 0.003, S * 0.006);
    g.filter = 'none'; g.globalCompositeOperation = 'source-in'; g.fillStyle = 'rgb(24,26,36)'; g.fillRect(0, 0, w, h);
    return cv;
  }
  EXTRA.push({ id: 'watercolor', name: 'Watercolour', blurb: 'A loose ring of ultramarine washes laid straight on the map, pooled dark at the edges, with blooms and splatter.', key: [64, 88, 170], luma: 0.85,
    spec: { roundOnly: true, rim: 0.12, inset: function (G) { return G.bs * 0.128; },
      front: function (ctx, G) {
        var pale = GRADE ? smooth(0.55, 0.9, yiq(GRADE.target)[0]) : 0;
        if (pale > 0.02) { ctx.save(); ctx.globalAlpha = 0.45 * pale; ctx.drawImage(masterLayer('watercolor-shade', G.outW, G.outH, watercolorShade), 0, 0); ctx.restore(); }
        ctx.drawImage(graded('watercolor-paint', masterLayer('watercolor-paint', G.outW, G.outH, watercolorPaint)), 0, 0);
      } } });
  /* ---------- Whirlpool: water swirling inward round the picture, seen from above ----------
     The water is a funnel: shallow and pale green-blue where it comes in over the map, falling away to deep, dark blue
     where it meets the picture. Its surface is baked once into textures: broad swells and foam laid out along the flow
     (across the spiral arms and along them), so every streak is already stretched with the current, and finer ones laid on
     the plain ring for the round bubbles of the foam and the crossing chop. Each frame the layers slide along the spiral at
     different speeds, so the foam drifts, stretches and breaks up against the ripples the way layers of real water do.
     Every pixel of a small fixed-size field is then lit: sky mirrored in the ripple faces, the bottom showing through,
     white foam on top with lace at its edges. The field is worked out once per frame whatever the grade; only the
     colouring is done per grade. */
  var WHIRLPOOL_N = 300, WHIRLPOOL_U = 512, WHIRLPOOL_V = 256, WHIRLPOOL_FU = 1024, WHIRLPOOL_FV = 128, WHIRLPOOL_K = 0.55, WHIRLPOOL_R0 = 0.43, WHIRLPOOL_ST = 13;
  var whirlpoolBaked = null, whirlpoolFld = null;

  // Value noise that repeats in both directions (px, py cells, whole numbers), with its own offsets so layers differ.
  function whirlpoolPN(x, y, px, py, ox, oy) {
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi;
    var x0 = ((xi % px) + px) % px, x1 = x0 + 1 === px ? 0 : x0 + 1, y0 = ((yi % py) + py) % py, y1 = y0 + 1 === py ? 0 : y0 + 1;
    var a = lat(x0 + ox, y0 + oy), b = lat(x1 + ox, y0 + oy), c = lat(x0 + ox, y1 + oy), d = lat(x1 + ox, y1 + oy);
    var u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
    return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v;
  }
  // Cellular noise that repeats in both directions (px, py cells, any whole numbers): distance to the nearest cell point and
  // to the second nearest, and the nearest cell's id.
  var whirlpoolVC = { f1: 0, f2: 0, id: 0 };
  function whirlpoolVor(x, y, px, py, ox, oy) {
    var xi = Math.floor(x), yi = Math.floor(y), f1 = 9, f2 = 9, id = 0;
    for (var j = -1; j <= 1; j++) for (var i = -1; i <= 1; i++) {
      var gx = xi + i, gy = yi + j, hx = (((gx % px) + px) % px) + ox, hy = (((gy % py) + py) % py) + oy, cx = gx + lat(hx, hy), cy = gy + lat(hy + 97, hx + 41);
      var d = Math.hypot(cx - x, cy - y);
      if (d < f1) { f2 = f1; f1 = d; id = lat(hx + 13, hy + 177); } else if (d < f2) f2 = d;
    }
    whirlpoolVC.f1 = f1; whirlpoolVC.f2 = f2; whirlpoolVC.id = id; return whirlpoolVC;
  }

  // The textures. u is a turn round the ring and w runs out along it (log of the radius). R and C are laid on the spiral, so a
  // feature narrow in u is a streak along an arm. F and W are laid on the plain ring, finer round it, so bubbles and chop are as wide as they are long. Every
  // noise repeats exactly over the texture, so the layers can slide round and out without a seam. Four channels to a texel, so
  // one lookup serves a whole layer.
  //   R: slope of the swell (2), a cloud of foam density, rough water
  //   W: slope of the chop (2), where the chop is strong
  //   C: ribbons of foam, a second cloud, thin threads, a ragged edge
  //   F: a foam's bubbles: cell id, distance between cells, dome, a cluster noise
  function whirlpoolBake() {
    var U = WHIRLPOOL_U, V = WHIRLPOOL_V, n = U * V, K = WHIRLPOOL_K, PN = whirlpoolPN, iu, iv, i;
    var TR = new Float32Array(n * 4), TC = new Float32Array(n * 4), h1 = new Float32Array(n);
    for (iv = 0; iv < V; iv++) for (iu = 0; iu < U; iu++) {
      i = iv * U + iu;
      var u = iu / U, w = iv / V;
      // the current bends everything laid along it: a wide slow bend and a tighter one
      var wa = PN(u * 16, w * 4, 16, 4, 0, 31) - 0.5 + 0.6 * (PN(u * 64, w * 16, 64, 16, 17, 3) - 0.5);
      var wb = PN(u * 16, w * 4, 16, 4, 9, 47) - 0.5 + 0.6 * (PN(u * 64, w * 16, 64, 16, 23, 61) - 0.5);
      // the swell: broad undulations stretched along the current
      h1[i] = 0.5 * PN(u * 16 + wa * 1.5, w * 16 + wb * 1.2, 16, 16, 0, 0) + 0.35 * PN(u * 32 + wb * 2, w * 32 + wa * 1.4, 32, 32, 21, 17) + 0.15 * PN(u * 48 + wa * 2.5, w * 40 + wb * 1.5, 48, 40, 33, 25);
      TR[i * 4 + 3] = 0.55 * PN(u * 16 + wa, w * 8 + wb * 0.4, 16, 8, 5, 19) + 0.3 * PN(u * 32 + wa * 1.5, w * 16 + wb * 0.6, 32, 16, 51, 29) + 0.15 * PN(u * 64 + wa * 2, w * 32, 64, 32, 23, 37);
      // clouds of foam density laid along the flow, in two layers that will slide past each other
      TR[i * 4 + 2] = 0.5 * PN(u * 32 + wa * 2.4, w * 8 + wb * 1.2, 32, 8, 41, 3) + 0.3 * PN(u * 64 + wa * 3.4, w * 16 + wb * 1.6, 64, 16, 3, 71) + 0.2 * PN(u * 64 + wa * 5, w * 32 + wb * 2, 64, 32, 59, 83);
      TC[i * 4 + 1] = 0.5 * PN(u * 32 + wb * 2.4, w * 8 + wa * 1.2, 32, 8, 77, 11) + 0.3 * PN(u * 64 + wb * 3.4, w * 16 + wa * 1.6, 64, 16, 91, 5) + 0.2 * PN(u * 64 + wb * 5, w * 32 + wa * 2, 64, 32, 103, 37);
      // ribbons: six broad arms of foam round the ring and twelve thin ones, wandering, each swelling and narrowing along its length
      var s6 = u * 6 + 0.55 * (PN(u * 8, w * 4, 8, 4, 3, 5) - 0.5) + 0.3 * (PN(u * 16, w * 8, 16, 8, 19, 23) - 0.5), j = Math.floor(s6), x6 = s6 - j - 0.5;
      var hw = 0.03 + 0.24 * PN(w * 8, ((j % 6) + 6) % 6 + 0.5, 8, 8, 29, 13);
      var s12 = u * 12 + 0.7 * (PN(u * 16, w * 8, 16, 8, 37, 41) - 0.5) + 0.4 * (PN(u * 32, w * 16, 32, 16, 59, 7) - 0.5), j2 = Math.floor(s12), x12 = s12 - j2 - 0.5;
      var hw2 = 0.01 + 0.13 * smooth(0.35, 0.75, PN(w * 12, ((j2 % 12) + 12) % 12 * 0.5 + 0.5, 12, 8, 71, 17));
      TC[i * 4] = Math.max(1 - smooth(hw - 0.1, hw + 0.1, Math.abs(x6)), 0.75 * (1 - smooth(hw2 - 0.05, hw2 + 0.05, Math.abs(x12))));
      // threads: thin filaments, the contour lines of a smooth field, bent more than the clouds are
      TC[i * 4 + 2] = 1 - Math.abs(2 * PN(u * 32 + wa * 5, w * 8 + wb * 1.6, 32, 8, 51, 29) - 1);
      TC[i * 4 + 3] = 0.6 * PN(u * 16 + wa * 2, w * 16 + wb, 16, 16, 71, 53) + 0.4 * PN(u * 32 + wa * 3, w * 32 + wb * 1.5, 32, 32, 83, 61);
    }
    // slopes from the heights, stored as the slope round the ring and the slope out from it, scaled so a typical ripple has slope 1
    var ss = 0, hp, hq;
    for (iv = 0; iv < V; iv++) for (iu = 0; iu < U; iu++) {
      i = iv * U + iu;
      hp = (h1[iv * U + ((iu + 1) & (U - 1))] - h1[iv * U + ((iu - 1) & (U - 1))]) * 0.5 * U;
      hq = (h1[((iv + 1) & (V - 1)) * U + iu] - h1[((iv - 1) & (V - 1)) * U + iu]) * 0.5 * V;
      var gt = hp / TAU, gr = hq + K * hp;
      TR[i * 4] = gt; TR[i * 4 + 1] = gr; ss += gt * gt + gr * gr;
    }
    var k = 1 / Math.sqrt(ss / n);
    for (i = 0; i < n; i++) { TR[i * 4] *= k; TR[i * 4 + 1] *= k; }
    // the bubbles, a little under a fortieth of the token across
    var FU = WHIRLPOOL_FU, FV = WHIRLPOOL_FV, nf = FU * FV, TF = new Float32Array(nf * 4);
    for (iv = 0; iv < FV; iv++) for (iu = 0; iu < FU; iu++) {
      i = iv * FU + iu;
      var fu = iu / FU, fw = iv / FV, vo = whirlpoolVor(fu * 176, fw * 28, 176, 28, 5, 9);
      TF[i * 4] = vo.id; TF[i * 4 + 1] = Math.min(0.5, vo.f2 - vo.f1); TF[i * 4 + 2] = Math.min(0.7, vo.f1);
      TF[i * 4 + 3] = 0.6 * PN(fu * 64, fw * 10, 64, 10, 61, 7) + 0.4 * PN(fu * 128, fw * 20, 128, 20, 13, 97);
    }
    // the chop: short waves as wide as they are long (a turn of the ring is 2 pi times the texture's height in the same units,
    // hence about 6.3 cells round to one cell out), their slopes found from the heights as for the swell
    var TW = new Float32Array(nf * 4), hc = new Float32Array(nf);
    for (iv = 0; iv < FV; iv++) for (iu = 0; iu < FU; iu++) {
      i = iv * FU + iu;
      var cu = iu / FU, cw = iv / FV;
      var ca = PN(cu * 16, cw * 4, 16, 4, 0, 31) - 0.5 + 0.6 * (PN(cu * 64, cw * 16, 64, 16, 17, 3) - 0.5);
      var cb = PN(cu * 16, cw * 4, 16, 4, 9, 47) - 0.5 + 0.6 * (PN(cu * 64, cw * 16, 64, 16, 23, 61) - 0.5);
      hc[i] = 0.45 * PN(cu * 48 + ca * 3, cw * 8 + cb * 0.5, 48, 8, 41, 57) + 0.35 * PN(cu * 96 + cb * 5, cw * 15 + ca * 0.8, 96, 15, 53, 11) + 0.2 * PN(cu * 192 + ca * 6, cw * 31 + cb * 1.1, 192, 31, 67, 77);
      TW[i * 4 + 2] = 0.6 * PN(cu * 24 + ca * 2, cw * 4 + cb * 0.4, 24, 4, 11, 13) + 0.4 * PN(cu * 48 + cb * 3, cw * 8 + ca * 0.6, 48, 8, 29, 7);
    }
    var sc2 = 0;
    for (iv = 0; iv < FV; iv++) for (iu = 0; iu < FU; iu++) {
      i = iv * FU + iu;
      hp = (hc[iv * FU + ((iu + 1) & (FU - 1))] - hc[iv * FU + ((iu - 1) & (FU - 1))]) * 0.5 * FU;
      hq = (hc[((iv + 1) & (FV - 1)) * FU + iu] - hc[((iv - 1) & (FV - 1)) * FU + iu]) * 0.5 * FV;
      TW[i * 4] = hp / TAU; TW[i * 4 + 1] = hq; sc2 += TW[i * 4] * TW[i * 4] + TW[i * 4 + 1] * TW[i * 4 + 1];
    }
    k = 1 / Math.sqrt(sc2 / nf);
    for (i = 0; i < nf; i++) { TW[i * 4] *= k; TW[i * 4 + 1] *= k; }
    // bytes are enough for all of it and four times lighter on the cache; each channel is stored over its own range
    function pack(T, m, lo, hi) {
      var B = new Uint8Array(m * 4), sc = [], bi = [], ch, q;
      for (ch = 0; ch < 4; ch++) { sc[ch] = (hi[ch] - lo[ch]) / 255; bi[ch] = lo[ch]; }
      for (var e = 0; e < m; e++) for (ch = 0; ch < 4; ch++) { q = Math.round((T[e * 4 + ch] - lo[ch]) / sc[ch]); B[e * 4 + ch] = q < 0 ? 0 : q > 255 ? 255 : q; }
      return { t: B, s: sc, b: bi };
    }
    return { r: pack(TR, n, [-4, -4, 0, 0], [4, 4, 1, 1]), f: pack(TF, nf, [0, 0, 0, 0], [1, 0.5, 0.7, 1]), c: pack(TC, n, [0, 0, 0, 0], [1, 1, 1, 1]), w: pack(TW, nf, [-4, -4, 0, 0], [4, 4, 1, 1]) };
  }

  // Where each pixel sits (round the ring and out from it, in the spiral's own coordinates) and the profiles that
  // depend only on the radius. One row of WHIRLPOOL_ST numbers per pixel of the ring.
  function whirlpoolPixels() {
    var N = WHIRLPOOL_N, list = [], U = WHIRLPOOL_U, V = WHIRLPOOL_V, K = WHIRLPOOL_K;
    for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
      var X = (x + 0.5) / N - 0.5, Y = (y + 0.5) / N - 0.5, r = Math.hypot(X, Y);
      if (r < 0.364 || r > 0.492) continue;
      var th = Math.atan2(Y, X) / TAU + 0.5, q = Math.log(r / WHIRLPOOL_R0);
      list.push(y * N + x, (th + K * q) * U, q * V, X / r, Y / r, r,
        1 - smooth(0.38, 0.475, r),                                  // depth: deep at the picture, shallow outside
        smooth(0.364, 0.39, r) * (0.95 - 0.28 * smooth(0.42, 0.5, r)) * (1 - smooth(0.475, 0.489, r)),   // water's opacity: fades in over the picture's edge, thins outward
        smooth(0.372, 0.43, r),                                      // foam keeps off the picture's edge
        0.07 + 0.3 * Math.exp(-(r - 0.385) / 0.05),                  // slope of the funnel, steeper where it falls away
        0.2 * Math.exp(-Math.pow((r - 0.45) / 0.045, 2)) - 0.1 * smooth(0.46, 0.495, r),  // where foam gathers
        0.5 + 0.55 * (1 - smooth(0.4, 0.5, r)),                      // ripples are stronger toward the picture
        th * WHIRLPOOL_FU);
    }
    return new Float32Array(list);
  }

  function whirlpoolAlloc() {
    var N = WHIRLPOOL_N, px = whirlpoolPixels(), m = px.length / WHIRLPOOL_ST, OFF = 2 * N + 4;
    return { q: -1, px: px, m: m, OFF: OFF, fo: new Float32Array(N * N + 2 * OFF), ft: new Float32Array(N * N + 2 * OFF), fh: new Float32Array(N * N + 2 * OFF), th: new Float32Array(m), sp: new Float32Array(N * N + 2 * OFF),
      sh: new Float32Array(m), fb: new Float32Array(m), has: new Uint8Array(m), fr: new Float32Array(m), sk: new Float32Array(m), dp: new Float32Array(m), ae: new Float32Array(m),
      wa: new Float32Array(m), em: new Float32Array(m), ca: new Float32Array(m), cand: new Float32Array(3 * 400), nc: 0, S: new Float32Array(12) };
  }

  // smooth(a, b, x) with the width's reciprocal given, for the per-pixel work
  function whirlpoolSm(x, a, inv) { var t = (x - a) * inv; t = t < 0 ? 0 : t > 1 ? 1 : t; return t * t * (3 - 2 * t); }

  // The lit water and its foam for this moment, independent of colour.
  function whirlpoolField(t) {
    var q = Math.round(t * 30), F = whirlpoolFld;
    if (F && F.q === q) return F;
    if (!whirlpoolBaked) whirlpoolBaked = whirlpoolBake();
    if (!F) F = whirlpoolFld = whirlpoolAlloc();
    var B = whirlpoolBaked, TR = B.r.t, TF = B.f.t, TC = B.c.t, TW = B.w.t, PX = F.px, ST = WHIRLPOOL_ST, N = WHIRLPOOL_N, U = WHIRLPOOL_U, V = WHIRLPOOL_V, OFF = F.OFF, S = F.S;
    var FO = F.fo, SP = F.sp, SH = F.sh, FR = F.fr, SK = F.sk, DP = F.dp, AE = F.ae, WA = F.wa, FB = F.fb, HAS = F.has, FT = F.ft, TH = F.th, FH = F.fh;
    // each layer slides along the spiral at its own speed (texels per second round and along), the ribbons creep inward
    var uR = 0.012 * U * t, vR = 0.07 * V * t, vF = 0.12 * WHIRLPOOL_FV * t, uF = WHIRLPOOL_K * 0.12 * WHIRLPOOL_FU * t, vW = 0.07 * WHIRLPOOL_FV * t, uW = (0.012 + WHIRLPOOL_K * 0.07) * WHIRLPOOL_FU * t, uC = 0.03 * U * t, vC = 0.03 * V * t;
    var L0 = LIGHT[0], L1 = LIGHT[1], L2 = LIGHT[2], H0 = HALF[0], H1 = HALF[1], H2 = HALF[2];
    FO.fill(0); SP.fill(0); FT.fill(0); FH.fill(0);
    var SM = whirlpoolSm, UM = U - 1, VM = V - 1, FUM = WHIRLPOOL_FU - 1, FVM = WHIRLPOOL_FV - 1, K255 = 1 / 255, RS0 = 8 / 255, RB0 = -4;
    for (var k = 0, kk = 0; k < PX.length; k += ST, kk++) {
      var i = (PX[k] + OFF) | 0, pu = PX[k + 1], pv = PX[k + 2], c = PX[k + 3], s = PX[k + 4], r = PX[k + 5], D0 = PX[k + 6], aBase = PX[k + 7], fIn = PX[k + 8], fun = PX[k + 9], fenv = PX[k + 10], rg = PX[k + 11], thU = PX[k + 12];
      // the two spiral layers
      var fu = pu - uR, fw = pv + vR, f0 = Math.floor(fu), f1 = Math.floor(fw), du = fu - f0, dw = fw - f1;
      var i0 = f0 & UM, i1 = (i0 + 1) & UM, j0 = (f1 & VM) << 9, j1 = ((f1 + 1) & VM) << 9;
      var w00 = (1 - du) * (1 - dw), w10 = du * (1 - dw), w01 = (1 - du) * dw, w11 = du * dw;
      var ta = (j0 + i0) << 2, tb = (j0 + i1) << 2, tc2 = (j1 + i0) << 2, td = (j1 + i1) << 2;
      var g0 = (TR[ta] * w00 + TR[tb] * w10 + TR[tc2] * w01 + TR[td] * w11) * RS0 + RB0, g1 = (TR[ta + 1] * w00 + TR[tb + 1] * w10 + TR[tc2 + 1] * w01 + TR[td + 1] * w11) * RS0 + RB0;
      var cloudD = (TR[ta + 2] * w00 + TR[tb + 2] * w10 + TR[tc2 + 2] * w01 + TR[td + 2] * w11) * K255, rough = (TR[ta + 3] * w00 + TR[tb + 3] * w10 + TR[tc2 + 3] * w01 + TR[td + 3] * w11) * K255;
      fu = pu + uC; fw = pv + vC; f0 = Math.floor(fu); f1 = Math.floor(fw); du = fu - f0; dw = fw - f1;
      i0 = f0 & UM; i1 = (i0 + 1) & UM; j0 = (f1 & VM) << 9; j1 = ((f1 + 1) & VM) << 9;
      w00 = (1 - du) * (1 - dw); w10 = du * (1 - dw); w01 = (1 - du) * dw; w11 = du * dw;
      ta = (j0 + i0) << 2; tb = (j0 + i1) << 2; tc2 = (j1 + i0) << 2; td = (j1 + i1) << 2;
      var arm = (TC[ta] * w00 + TC[tb] * w10 + TC[tc2] * w01 + TC[td] * w11) * K255, cloudC = (TC[ta + 1] * w00 + TC[tb + 1] * w10 + TC[tc2 + 1] * w01 + TC[td + 1] * w11) * K255;
      var thread = (TC[ta + 2] * w00 + TC[tb + 2] * w10 + TC[tc2 + 2] * w01 + TC[td + 2] * w11) * K255, edge = (TC[ta + 3] * w00 + TC[tb + 3] * w10 + TC[tc2 + 3] * w01 + TC[td + 3] * w11) * K255;
      // the chop rides the same current as the swell; the surface is the two on the slope of the funnel, rougher in some lanes
      fu = thU - uW; fw = pv * 0.5 + vW; f0 = Math.floor(fu); f1 = Math.floor(fw); du = fu - f0; dw = fw - f1;
      i0 = f0 & FUM; i1 = (i0 + 1) & FUM; j0 = (f1 & FVM) << 10; j1 = ((f1 + 1) & FVM) << 10;
      w00 = (1 - du) * (1 - dw); w10 = du * (1 - dw); w01 = (1 - du) * dw; w11 = du * dw;
      ta = (j0 + i0) << 2; tb = (j0 + i1) << 2; tc2 = (j1 + i0) << 2; td = (j1 + i1) << 2;
      var c0 = (TW[ta] * w00 + TW[tb] * w10 + TW[tc2] * w01 + TW[td] * w11) * RS0 + RB0, c1 = (TW[ta + 1] * w00 + TW[tb + 1] * w10 + TW[tc2 + 1] * w01 + TW[td + 1] * w11) * RS0 + RB0;
      var cm = (TW[ta + 2] * w00 + TW[tb + 2] * w10 + TW[tc2 + 2] * w01 + TW[td + 2] * w11) * K255;
      var rs = 0.4 * (0.3 + 1.1 * rough) * rg, rc = 0.24 * (0.35 + 1.3 * cm) * rg, gt = g0 * rs + c0 * rc, gr = g1 * rs + c1 * rc + fun;
      var gx = gr * c - gt * s, gy = gr * s + gt * c, nz = 1 / Math.sqrt(1 + gx * gx + gy * gy), nx = -gx * nz, ny = -gy * nz;
      var dl = nx * L0 + ny * L1 + nz * L2;
      SH[kk] = dl < -0.0667 ? 0.4 : dl > 0.8 ? 1.6 : 1 + 1.3 * (dl - 0.466);
      var rl = 2 * nz * (nx * L0 + ny * L1) + (2 * nz * nz - 1) * L2;
      FR[kk] = 0.05 + 0.75 * Math.sqrt(gx * gx + gy * gy);
      SK[kk] = 0.35 + 0.65 * SM(rl, 0.1, 1 / 0.75);
      var dh = nx * H0 + ny * H1 + nz * H2;
      if (dh > 0.9) { var s2 = dh * dh, s4 = s2 * s2, s8 = s4 * s4, s16 = s8 * s8, s32 = s16 * s16, s64 = s32 * s32; SP[i] = s64 * s64; }
      var lim = 0.447 + 0.03 * (edge - 0.5) + 0.02 * (cloudC - 0.5);
      WA[kk] = aBase * (1 - SM(r, lim, 60));
      DP[kk] = D0 + 0.1 * (0.5 - rough) + 0.3 * (cloudD - 0.5);
      // foam: a density from clouds, ribbons and the rough water; each bubble cell is foam where the density beats its own
      // random level, so patches end in rounded bubbles and thin foam is a net of walls with the water showing in the cells
      var dens = ((cloudD + cloudC) * 0.5 - 0.5) * 1.5 + arm * 1.15 + (rough - 0.5) * 0.4 + fenv - 0.47;
      var fOut = r < 0.455 - (edge - 0.5) * 0.03 ? 1 : 1 - SM(r + (edge - 0.5) * 0.03, 0.455, 38.5);
      // streamers: thin filaments drawn along the flow where the water is rough, fading in and out along their length
      var fil = thread > 0.9 ? SM(thread, 0.9, 12.5) * SM(cloudD, 0.38, 4) * (0.5 + 0.5 * SM(arm + cloudC, 0.5, 1)) * 0.8 * fIn * fOut : 0;
      if (dens < -0.5 || fIn <= 0) {
        AE[kk] = dens < -0.9 ? 0 : SM(dens, -0.9, 0.77) * 0.7; TH[kk] = 0.15; FB[kk] = 0.92; HAS[kk] = 0;
        if (fil > 0.01) { FO[i] = fil; FT[i] = fil * 0.4; HAS[kk] = 1; }
        continue;
      }
      fu = thU - uF; fw = pv * 0.5 + vF; f0 = Math.floor(fu); f1 = Math.floor(fw); du = fu - f0; dw = fw - f1;
      i0 = f0 & FUM; i1 = (i0 + 1) & FUM; j0 = (f1 & FVM) << 10; j1 = ((f1 + 1) & FVM) << 10;
      w00 = (1 - du) * (1 - dw); w10 = du * (1 - dw); w01 = (1 - du) * dw; w11 = du * dw;
      ta = (j0 + i0) << 2; tb = (j0 + i1) << 2; tc2 = (j1 + i0) << 2; td = (j1 + i1) << 2;
      var cid = (TF[ta] * w00 + TF[tb] * w10 + TF[tc2] * w01 + TF[td] * w11) * K255, cd = (TF[ta + 1] * w00 + TF[tb + 1] * w10 + TF[tc2 + 1] * w01 + TF[td + 1] * w11) * (0.5 * K255);
      var cdome = (TF[ta + 2] * w00 + TF[tb + 2] * w10 + TF[tc2 + 2] * w01 + TF[td + 2] * w11) * (0.7 * K255), clus = (TF[ta + 3] * w00 + TF[tb + 3] * w10 + TF[tc2 + 3] * w01 + TF[td + 3] * w11) * K255;
      HAS[kk] = 1;
      var dc = dens + 0.55 * (0.5 - cid) + 0.25 * (clus - 0.5), inner = SM(dc, 0.3, 5.5), crev = 1 - SM(cd, 0.03, 7), wall = SM(dc + 0.2, -0.1, 3) * (1 - SM(cd, 0.04, 6)) * SM(clus, 0.38, 5);
      var fo = Math.max(Math.max(inner, wall * 0.9) * (0.8 + 0.2 * SM(dc, 0.3, 1.43)) * fIn * fOut, fil);
      FO[i] = fo;
      var th = SM(dc, 0.1, 1);
      TH[kk] = th; FT[i] = fo * (0.35 + 0.65 * th); FH[i] = fo * (0.55 * (1 - SM(cdome, 0, 2)) + 0.45 * clus);
      FB[kk] = 0.88 + 0.1 * (1 - SM(cdome, 0, 2)) + 0.3 * (clus - 0.5) + 0.25 * (thread - 0.5) - 0.3 * crev;
      AE[kk] = SM(dens, -0.9, 0.77) * 0.7;
    }
    // foam is lit like the heap it is: bright on the side that faces the light, and it shades the water beside it
    var EM = F.em, CA = F.ca, nc = 0, CAND = F.cand;
    for (k = 0, kk = 0; k < PX.length; k += ST, kk++) {
      i = (PX[k] + OFF) | 0;
      if (HAS[kk]) {
        EM[kk] = 0.3 * (FO[i + 1] - FO[i - 1]) + 0.4 * (FO[i + N] - FO[i - N]) + 0.3 * (FT[i + 2] - FT[i - 2]) + 0.4 * (FT[i + 2 * N] - FT[i - 2 * N]) + 0.8 * (FH[i + 1] - FH[i - 1]) + 1.0 * (FH[i + N] - FH[i - N]);
        CA[kk] = Math.max(FO[i - N - 1], FO[i - 2 * N - 2]) * (1 - FO[i]);
      } else { EM[kk] = 0; CA[kk] = 0; }
      var sp = SP[i];
      if (sp > 0.6 && nc < 400 && sp >= SP[i - 1] && sp >= SP[i + 1] && sp >= SP[i - N] && sp >= SP[i + N] && FO[i] < 0.5 && WA[kk] > 0.6) { CAND[nc * 3] = i - OFF; CAND[nc * 3 + 1] = sp; CAND[nc * 3 + 2] = kk; nc++; }
    }
    F.nc = nc; F.q = q; return F;
  }

  // The water's colours under the grade in force, with the shallow-to-deep ramp worked out once.
  var whirlpoolPals = {};
  function whirlpoolPal() {
    var k = gradeId();
    if (!whirlpoolPals[k]) {
      var P = whirlpoolPals[k] = {
        shallow: tc([70, 184, 160]), mid: tc([12, 92, 122]), deep: tc([3, 26, 50]), glow: tc([120, 214, 190]), sky: tc([150, 196, 224]),
        foam: tc([242, 249, 250]), foamShade: tc([150, 196, 206]), glint: tc([232, 246, 255]), shadow: tc([2, 18, 30]), lut: new Float32Array(129 * 3)
      };
      for (var j = 0; j <= 128; j++) {
        var D = j / 128, A0 = D < 0.5 ? P.shallow : P.mid, A1 = D < 0.5 ? P.mid : P.deep, e = D < 0.5 ? D * 2 : D * 2 - 1;
        for (var c = 0; c < 3; c++) P.lut[j * 3 + c] = A0[c] + (A1[c] - A0[c]) * e;
      }
    }
    return whirlpoolPals[k];
  }
  var whirlpoolTex = {};
  function whirlpoolTexture(t) {
    var F = whirlpoolField(t), gk = gradeId(), c = whirlpoolTex[gk], N = WHIRLPOOL_N;
    if (c && c.q === F.q) return c.cv;
    if (!c) { c = whirlpoolTex[gk] = { cv: document.createElement('canvas') }; c.cv.width = c.cv.height = N; c.img = c.cv.getContext('2d').createImageData(N, N); }
    var P = whirlpoolPal(), PX = F.px, ST = WHIRLPOOL_ST, OFF = F.OFF, d = c.img.data, FO = F.fo, SP = F.sp, LUT = P.lut;
    var DPa = F.dp, WAa = F.wa, THa = F.th, SHa = F.sh, AEa = F.ae, FRa = F.fr, SKa = F.sk, CAa = F.ca, FBa = F.fb, EMa = F.em;
    var sky0 = P.sky[0], sky1 = P.sky[1], sky2 = P.sky[2], gl0 = P.glow[0], gl1 = P.glow[1], gl2 = P.glow[2], sd0 = P.shadow[0], sd1 = P.shadow[1], sd2 = P.shadow[2];
    var gt0 = P.glint[0], gt1 = P.glint[1], gt2 = P.glint[2], fm0 = P.foam[0], fm1 = P.foam[1], fm2 = P.foam[2];
    var fs0 = P.foamShade[0], fs1 = P.foamShade[1], fs2 = P.foamShade[2], fd0 = fm0 - fs0, fd1 = fm1 - fs1, fd2 = fm2 - fs2;
    for (var k = 0, kk = 0; k < PX.length; k += ST, kk++) {
      var i = PX[k] | 0, o = i << 2, aw = WAa[kk], th = THa[kk], fo = FO[i + OFF];
      var fal = fo * (0.8 + 0.2 * th); if (fal > 1) fal = 1;
      var ta = 1 - (1 - aw) * (1 - fal);
      if (ta <= 0.002) { d[o + 3] = 0; continue; }
      var sh = SHa[kk], ae = AEa[kk] * 0.5, li = ((DPa[kk] < 0 ? 0 : DPa[kk] > 1 ? 1 : DPa[kk]) * 128 + 0.5) | 0, li3 = li * 3, fre = FRa[kk], sb = SKa[kk], ca = CAa[kk] * 0.5, sp = SP[i + OFF]; sp = sp < 0.4 ? 0 : (sp - 0.4) * 0.5;
      var wr = LUT[li3] * sh, wg = LUT[li3 + 1] * sh, wb = LUT[li3 + 2] * sh;
      wr += (sky0 * sb - wr) * fre; wg += (sky1 * sb - wg) * fre; wb += (sky2 * sb - wb) * fre;
      wr += (gl0 - wr) * ae; wg += (gl1 - wg) * ae; wb += (gl2 - wb) * ae;
      wr += (sd0 - wr) * ca + gt0 * sp; wg += (sd1 - wg) * ca + gt1 * sp; wb += (sd2 - wb) * ca + gt2 * sp;
      var lit = FBa[kk] + 0.45 * EMa[kk] - 0.14 * (1 - th); lit = lit < 0 ? 0 : lit > 1 ? 1 : lit;
      var wa2 = aw * (1 - fal), inv = 1 / ta;
      d[o] = (wr * wa2 + (fs0 + fd0 * lit) * fal) * inv; d[o + 1] = (wg * wa2 + (fs1 + fd1 * lit) * fal) * inv; d[o + 2] = (wb * wa2 + (fs2 + fd2 * lit) * fal) * inv; d[o + 3] = 255 * ta + 0.5;
    }
    c.cv.getContext('2d').putImageData(c.img, 0, 0); c.q = F.q;
    return c.cv;
  }

  // Glints: where a ripple face tips straight at the light the sky shows as a small hard spark, drawn at the canvas's own
  // resolution to stay sharp. They are the peaks of the lit field, so they ride the ripples round with the flow.
  var whirlpoolGlintSprites = {};
  function whirlpoolGlintSprite() {
    var gk = gradeId(), s = whirlpoolGlintSprites[gk];
    if (s) return s;
    var c = document.createElement('canvas'); c.width = c.height = 32;
    var g = c.getContext('2d'), P = whirlpoolPal(), gr = g.createRadialGradient(16, 16, 0, 16, 16, 16);
    gr.addColorStop(0, rgba(P.glint, 1)); gr.addColorStop(0.18, rgba(P.glint, 0.7)); gr.addColorStop(0.5, rgba(P.sky, 0.18)); gr.addColorStop(1, rgba(P.sky, 0));
    g.fillStyle = gr; g.fillRect(0, 0, 32, 32);
    whirlpoolGlintSprites[gk] = c; return c;
  }
  function whirlpoolGlints(ctx, G, F) {
    var T = G.outW, k = T / WHIRLPOOL_N, C = F.cand, n = F.nc, N = WHIRLPOOL_N, spr = whirlpoolGlintSprite(), PX = F.px, ST = WHIRLPOOL_ST;
    ctx.save(); ctx.globalCompositeOperation = 'lighter';
    for (var i = 0; i < n; i++) {
      var amp = smooth(0.6, 0.98, C[i * 3 + 1]), ix = C[i * 3], x = (ix % N + 0.5) * k, y = (Math.floor(ix / N) + 0.5) * k, kk = C[i * 3 + 2];
      var cs = PX[kk * ST + 3], sn = PX[kk * ST + 4], sz = T * (0.0045 + 0.006 * amp), a = sz * 1.5, b = sz * 0.6;
      // a streak along the flow, which runs tangent to the ring: the sprite's long side points along (-sn, cs)
      ctx.globalAlpha = Math.min(1, amp * 1.3);
      ctx.setTransform(-sn * a / 16, cs * a / 16, -cs * b / 16, -sn * b / 16, x, y);
      ctx.drawImage(spr, -16, -16);
    }
    ctx.restore();
  }
  // Spray: drops flung off the foam where the current is fastest. Each leaves along the line its water was travelling, with a
  // little outward push, so it flies off tangent to the ring, blurs with speed, fades and is gone.
  function whirlpoolSpray(ctx, G, t, F) {
    var T = G.outW, N = WHIRLPOOL_N, cx = T / 2, cy = G.outH / 2, P = whirlpoolPal(), FO = F.fo, OFF = F.OFF;
    ctx.save(); ctx.lineCap = 'round';
    for (var i = 0; i < 40; i++) {
      var life = 0.9 + 0.9 * hash(i, 2), cyc = t / life + hash(i, 1), ph = fract(cyc), id = i * 53 + Math.floor(cyc) * 11, tau = ph * life;
      var r0 = 0.4 + 0.05 * hash(id, 3), a0 = hash(id, 4) * TAU;
      var fx = Math.floor((0.5 + Math.cos(a0) * r0) * N), fy = Math.floor((0.5 + Math.sin(a0) * r0) * N);
      var foam = FO[fy * N + fx + OFF];
      var env = smooth(0, 0.1, ph) * (1 - smooth(0.5, 1, ph)) * smooth(0.05, 0.4, foam);
      if (env <= 0.01) continue;
      var ex = Math.cos(a0), ey = Math.sin(a0), vt = 0.12 * (0.9 + 0.3 * hash(id, 5)), vr = 0.03 + 0.05 * hash(id, 6);
      var x = cx + T * (ex * r0 - ey * vt * tau + ex * vr * tau), y = cy + T * (ey * r0 + ex * vt * tau + ey * vr * tau);
      var x0 = cx + T * (ex * r0 - ey * vt * (tau - 0.06) + ex * vr * (tau - 0.06)), y0 = cy + T * (ey * r0 + ex * vt * (tau - 0.06) + ey * vr * (tau - 0.06));
      var rr = Math.hypot(x - cx, y - cy) / T, edgeFade = 1 - smooth(0.445, 0.478, rr), rad = T * (0.003 + 0.0045 * hash(id, 7)) * (0.9 + 0.4 * ph);
      env *= edgeFade; if (env <= 0.01) continue;
      ctx.strokeStyle = rgba(P.foam, 0.3 * env); ctx.lineWidth = rad * 1.5; ctx.beginPath(); ctx.moveTo(x0, y0); ctx.lineTo(x, y); ctx.stroke();
      ctx.fillStyle = rgba(P.foam, 0.55 * env); ctx.beginPath(); ctx.arc(x, y, rad, 0, TAU); ctx.fill();
      ctx.fillStyle = rgba(P.glint, 0.95 * env); ctx.beginPath(); ctx.arc(x - rad * 0.35, y - rad * 0.4, rad * 0.32, 0, TAU); ctx.fill();
    }
    ctx.restore();
  }
  function whirlpoolFront(ctx, G, t) {
    var tex = whirlpoolTexture(t), F = whirlpoolFld;
    ctx.save(); ctx.imageSmoothingQuality = 'high'; ctx.drawImage(tex, 0, 0, G.outW, G.outH); ctx.restore();
    whirlpoolGlints(ctx, G, F);
    whirlpoolSpray(ctx, G, t, F);
  }
  EXTRA.push({ id: 'whirlpool', name: 'Whirlpool', blurb: 'Water swirling round the token, deep and dark where it falls away, foam streaming along the current and breaking up over the map.', key: [30, 110, 140], luma: 0.2, spread: 0.6,
    spec: { anim: true, roundOnly: true, rim: 0.115, inset: function (G) { return G.bs * 0.115; }, front: whirlpoolFront } });

  /* ---------- The set ---------- */
  function plainBack(ctx, G) {
    if (S.clear) return; // the shadow is under the picture; with no background it would show as a grey disc
    ctx.save(); ctx.filter = 'blur(' + Math.max(1, G.bs * 0.012) + 'px)'; ctx.fillStyle = 'rgba(0,0,0,.4)';
    ctx.translate(G.ox + G.bs * 0.006, G.oy + G.bs * 0.012); shapePath(ctx, G.bw, G.bh, G.bs * 0.012, G.isRound); ctx.fill(); ctx.restore();
  }
  function plainFront(ctx, G) { // a fine dark edge with a faint light line inside it, so the cut looks clean, not pasted
    ctx.save(); ctx.translate(G.ox, G.oy);
    shapePath(ctx, G.bw, G.bh, G.bs * 0.012, G.isRound); ctx.lineWidth = Math.max(1, G.bs * 0.004); ctx.strokeStyle = 'rgba(0,0,0,.55)'; ctx.stroke();
    shapePath(ctx, G.bw, G.bh, G.bs * 0.016, G.isRound); ctx.lineWidth = Math.max(0.6, G.bs * 0.002); ctx.strokeStyle = 'rgba(255,255,255,.16)'; ctx.stroke();
    ctx.restore();
  }
  function stoneInset(G) { return G.rw * 0.55; } // chips at the inner edge show the picture, not the map
  function inkInset(G) { return G.bs * 0.128; }
  function inkPaper(kind) {
    return { roundOnly: true, rim: 0.12, inset: inkInset, back: function (ctx, G) { washi(ctx, G.outW, G.outH, kind); },
      front: function (ctx, G) { ctx.drawImage(graded('ink', inkLayer(G.bw, G.bh)), G.ox, G.oy); ctx.drawImage(sealLayer(G.bw, G.bh), G.ox, G.oy); } };
  }
  function smoke(kind) {
    return { anim: true, roundOnly: true, rim: 0.08, feather: 0.035,
      front: function (ctx, G, t) { ctx.save(); ctx.imageSmoothingQuality = 'high'; ctx.drawImage(shadowTexture(kind, t), 0, 0, G.outW, G.outH); ctx.restore(); } };
  }
  function fire(kind) { return { anim: true, roundOnly: true, scale: 0.84, mat: IRON, rimId: 'iron', front: fireFront(kind) }; }
  function storybook(kind) {
    return { anim: true, roundOnly: true, rim: 0.11, inset: function (G) { return G.bs * 0.11; }, back: paperBack,
      front: function (ctx, G, t) { paperFront(ctx, G, t, playing && !G.still, STORYBOOKS[kind]); } };
  }
  // Six groups, thirty designs. Every design is its own idea: colour is the colour control's job, so no two are
  // the same design in different colours. key is a design's own main colour and luma how far its lightness
  // follows a new one (see Recolouring).
  var GROUPS = [
    { id: 'stone', name: 'Stone and glass', line: 'Carved, cut and polished, and glass with the light coming through.', ids: ['stone', 'granite', 'marble', 'obsidian', 'snowflake', 'mahogany', 'stainedglass'] },
    { id: 'crafted', name: 'Hide, metal and rope', line: 'Made by hand and worn by years of use.', ids: ['leather', 'bronze', 'rustiron', 'chain', 'rope', 'scales'] },
    { id: 'ink', name: 'Ink, paint and paper', line: 'Drawn, painted and cut by hand.', ids: ['ink', 'inkring', 'watercolor', 'paper'] },
    { id: 'elements', name: 'Elements', line: 'Fire, storm, ice, molten rock, acid and water, all moving.', ids: ['fire', 'storm', 'blizzard', 'lava', 'acid', 'whirlpool'] },
    { id: 'magic', name: 'Magic', line: 'Arcane, prismatic, holy and starlit light.', ids: ['arcane', 'prismatic', 'divine', 'astral'] },
    { id: 'spirits', name: 'Spirits', line: 'Smoke, mist and gas with no ring at all.', ids: ['shadow', 'ghost', 'poison'] }
  ];
  function design(id, name, blurb, key, luma, spec) { spec.id = id; spec.name = name; spec.blurb = blurb; spec.key = key; spec.luma = luma; return spec; }
  var DESIGNS = [
    design('stone', 'Carved stone', 'Pale limestone cut by a mason: calm, finely grained faces, the edges worn back and chipped, a hairline crack or two.', [176, 150, 112], 0.5, { mat: SANDSTONE, inset: stoneInset }),
    design('granite', 'Granite', 'Polished pink and grey granite speckled with black, the light of the room caught in its shine.', [150, 128, 122], 0.5, { mat: GRANITE, inset: stoneInset }),
    design('marble', 'Marble', 'Polished white marble with soft grey veins through it and a chip or two off the outer edge.', [216, 210, 198], 0.5, { mat: MARBLE, inset: stoneInset }),
    design('obsidian', 'Obsidian', 'Black glass with a faint rainbow sheen under the polish.', [18, 18, 22], 0.25, { mat: OBSIDIAN }),
    design('snowflake', 'Snowflake obsidian', 'Black glass with grey-white crystal flowers grown inside it.', [200, 200, 196], 0.2, { mat: SNOWFLAKE }),
    design('mahogany', 'Mahogany obsidian', 'Black glass swirled through with red-brown, the way it flowed as it cooled.', [120, 48, 26], 0.3, { mat: MAHOGANY }),
    design('leather', 'Adventurer leather', 'Tan, brown and red hides, smooth, pebbled and suede, one seam laced with a thong.', [150, 96, 58], 0.5, { mat: LEATHER }),
    design('bronze', 'Weathered bronze', 'Old bronze gone green and powdery in the recesses, pitted and scratched.', [140, 92, 56], 0.3, { mat: BRONZE }),
    design('rustiron', 'Rusted iron', 'Dark iron with rust blooming through, pitted where it has eaten in.', [156, 80, 36], 0.3, { mat: RUSTIRON }),
    design('ink', 'Ink on washi', 'One dry-brush circle on cream washi paper, with a vermilion seal.', [14, 14, 20], 1, inkPaper('washi')),
    design('fire', 'Ring of fire', 'Burning all the way round, the flames streaming outward, embers flung off and ash drifting away.', [242, 112, 22], 0, Object.assign(fire('orange'), { spread: 0.2 })),
    design('shadow', 'Living shadow', 'No ring at all: the ring is the smoke itself, rolling round and spilling over the edges.', [5, 6, 9], 0, smoke('shadow')),
    design('paper', 'Paper storybook', 'A hand-inked paper cut-out with a waving name banner. Set it to still and it is a paper token.', [143, 42, 42], 0.5, storybook('cream'))
  ];
  // Designs defined in their own files add themselves to EXTRA as { id, name, blurb, key, luma, spec }.
  EXTRA.forEach(function (e) { if (e.spread != null) e.spec.spread = e.spread; DESIGNS.push(design(e.id, e.name, e.blurb, e.key, e.luma, e.spec)); });
  var BY = {}; DESIGNS.forEach(function (d) { BY[d.id] = d; });
  GROUPS.forEach(function (g) { g.members = g.ids.filter(function (id) { return BY[id]; }).map(function (id) { BY[id].group = g.id; return BY[id]; }); });
  var NONE = { id: 'none', name: 'Plain circle', blurb: 'Just the picture, cut to a clean circle (or square) with a fine edge and a soft shadow.', group: 'none',
    inset: function (G) { return G.bs * 0.012; }, back: plainBack, front: plainFront };
  DESIGNS.push(NONE); BY.none = NONE;
  var GRP = {}; GROUPS.forEach(function (g) { GRP[g.id] = g; });
  // Square frames only exist for the shaded materials; the picture frame has to be rectangular.
  function fitsShape(D, mode) { return mode === 'picture' ? !D.roundOnly && !D.anim : true; }

  /* ---------- One framed result into any canvas ---------- */
  function art(ctx, src, p, k) {
    var fr = p.fr;
    ctx.save();
    ctx.translate((p.img.x - (fr.x - fr.w / 2)) * k, (p.img.y - (fr.y - fr.h / 2)) * k);
    if (S.flip) ctx.scale(-1, 1);
    var dw = iw() * p.img.s * k, dh = ih() * p.img.s * k;
    ctx.imageSmoothingQuality = 'high';
    ctx.drawImage(src, -dw / 2, -dh / 2, dw, dh);
    ctx.restore();
  }
  var grades = new Map();
  function gradeFor(D, colour) {
    if (!colour || !D.key) return null;
    var k = D.id + '|' + colour;
    if (!grades.has(k)) grades.set(k, makeGrade(D.key, rgb(colour), D.luma, D.spread));
    return grades.get(k);
  }
  var featherCv = new Map();
  // One per size in use, so the editor, the map tokens and the set below don't reallocate a shared one in turn.
  function featherCanvas(w, h) {
    var k = w + 'x' + h, cv = featherCv.get(k);
    if (!cv) {
      if (featherCv.size >= 8) featherCv.delete(featherCv.keys().next().value);
      cv = document.createElement('canvas'); cv.width = w; cv.height = h;
    } else featherCv.delete(k);
    featherCv.set(k, cv);
    return cv;
  }
  // still: draw an animated design as its still version (the token's own "animated" setting is off).
  // colour: the creature's colour for the frame, or nothing for the design's own colours.
  function render(ctx, outW, outH, mode, id, t, still, colour) {
    if (!S.img || !S[mode]) return;
    var keep = S.mode; S.mode = mode;
    var D = BY[id] || BY.none, p = P(), k = outW / p.fr.w, isRound = D.roundOnly ? true : round(), time = still ? 2.5 : t || 0;
    var sc = D.scale || 1, bw = Math.round(outW * sc), bh = Math.round(outH * sc), bs = Math.min(bw, bh);
    var G = { outW: outW, outH: outH, bw: bw, bh: bh, ox: (outW - bw) / 2, oy: (outH - bh) / 2, bs: bs, isRound: isRound, still: !!still,
      rw: D.mat ? D.mat.width * bs : (D.rim || 0) * bs };
    GRADE = gradeFor(D, colour);
    ctx.save(); ctx.clearRect(0, 0, outW, outH);
    if (D.back) D.back(ctx, G, time);
    var inset = D.inset ? D.inset(G) : G.rw ? G.rw - 1 : 0;
    // The picture is put together on a canvas of its own and cut out once: cutting the dark backing and the picture
    // separately leaves a dark hairline at the cut, and fading inside a clip leaves a ring of the clip's soft edge.
    var pc = featherCanvas(outW, outH), px = pc.getContext('2d');
    px.globalCompositeOperation = 'source-over'; px.clearRect(0, 0, outW, outH);
    // A picture whose background was removed keeps it see-through: no backing.
    if (!S.clear) { px.fillStyle = '#0c0c0d'; px.fillRect(0, 0, outW, outH); }
    art(px, S.img, p, k);
    if (D.feather) { // smoke is see-through, so the picture fades into it instead of ending at a hard circle
      var Rin = bs / 2 - inset, fg = px.createRadialGradient(outW / 2, outH / 2, Rin - bs * D.feather, outW / 2, outH / 2, Rin);
      fg.addColorStop(0, '#000'); fg.addColorStop(1, 'rgba(0,0,0,0)');
      px.globalCompositeOperation = 'destination-in'; px.fillStyle = fg; px.fillRect(0, 0, outW, outH);
    } else {
      ctx.save(); ctx.translate(G.ox, G.oy); shapePath(ctx, bw, bh, inset, isRound); ctx.restore(); ctx.clip();
    }
    ctx.drawImage(pc, 0, 0);
    ctx.restore(); ctx.save();
    if (D.mat) { var rim = rimLayer(D.rimId || D.id, bw, bh, isRound, D.mat); ctx.drawImage(D.rimId ? rim : graded('rim|' + D.id, rim), G.ox, G.oy); }
    if (D.front) D.front(ctx, G, time);
    ctx.restore();
    GRADE = null; S.mode = keep;
  }

  /* ---------- What the token maker reads ---------- */
  // playing tells the hand-drawn designs whether to boil their lines; the
  // maker turns it off under Calm, Off and reduced motion.
  var playing = true;
  function copyPlace(p) { return { img: { x: p.img.x, y: p.img.y, s: p.img.s }, fr: { x: p.fr.x, y: p.fr.y, w: p.fr.w, h: p.fr.h } }; }
  window.ChronicleTokenFrames = {
    W: W,
    MASTER: MASTER,
    groups: GROUPS.map(function (g) { return { id: g.id, name: g.name, line: g.line, ids: g.ids.slice() }; }),
    // Every design, the plain circle last; the picker's previews are drawn in this order.
    designs: DESIGNS.map(function (d) { return { id: d.id, name: d.name, blurb: d.blurb, anim: !!d.anim, roundOnly: !!d.roundOnly }; }),
    // place is the starting placement for a picture: the frame on the work
    // surface and the picture filling it. Placement is always the person's;
    // this is only where it starts.
    place: function (img) {
      var keep = S.img; S.img = img; S.mode = 'token';
      startPlacement('token'); fillFrame();
      var p = copyPlace(S.token); S.img = keep; return p;
    },
    // render draws one token into ctx, w by h pixels. o: img (the picture),
    // place (from place, as moved), frame (a design id), colour ('' or
    // '#rrggbb'), shape ('round' or 'square'), flip, name (the storybook's
    // banner), t (seconds), still (draw a moving design as its still picture)
    // and clear (the picture's background was removed, so nothing is put behind it).
    render: function (ctx, w, h, o) {
      if (!o || !o.img || !o.place) return;
      S.img = o.img; S.flip = !!o.flip; S.clear = !!o.clear; S.shape = o.shape === 'square' ? 'square' : 'round'; S.token = o.place; S.mode = 'token'; S.name = o.name || '';
      render(ctx, w, h, 'token', BY[o.frame] ? o.frame : 'none', o.t || 0, !!o.still, o.colour || '');
    },
    motion: function (on) { playing = !!on; },
    // The painted sample figure, for the previews tool.
    sample: function () { return sampleArt().full; }
  };
})();
