/**
 * token_cutout.js -- the token maker's background remover
 * (window.ChronicleTokenCutout). Art tools asked for "no background" often
 * paint a checkerboard instead, so the token would not be see-through on the
 * map. This finds a painted checkerboard or flat backdrop and works out which
 * pixels to clear.
 *
 * It is pure pixel work on { data, width, height } (an ImageData or the same
 * shape), with no DOM, so the token maker loads it on first open and the node
 * tests run it directly. The token maker keeps the picture and draws the
 * cleared copy; the page's own picture is never changed.
 *
 *   detect(px)                  -> { kind, sure, colours }
 *   mask(px, bg, strength)      -> Uint8Array, one alpha per pixel
 *   compose(px, bg, auto, paint, out, x0, y0, x1, y1)
 */
(function (root) {
  'use strict';

  // How far, in RGB distance, a pixel may be from a backdrop colour and still
  // be cleared. Medium cleanly clears the checkerboards art tools paint.
  var TOL = { light: 22, medium: 40, strong: 62 };
  // How near a border pixel must be to a backdrop colour to count as backdrop.
  var FIT = 30;

  function lum(c) { return 0.299 * c[0] + 0.587 * c[1] + 0.114 * c[2]; }
  function dist(r, g, b, c) { var dr = r - c[0], dg = g - c[1], db = b - c[2]; return Math.sqrt(dr * dr + dg * dg + db * db); }

  // The picture's outer band, sampled: the backdrop, if there is one, is what
  // runs all the way round the edge.
  function border(px) {
    var w = px.width, h = px.height, d = px.data, rgb = [], clear = 0, n = 0;
    var band = Math.max(2, Math.round(Math.min(w, h) * 0.015)), step = Math.max(1, Math.round(Math.max(w, h) / 600));
    function take(x, y) {
      var i = (y * w + x) * 4;
      n++;
      if (d[i + 3] < 200) { clear++; return; }
      rgb.push(d[i], d[i + 1], d[i + 2]);
    }
    for (var y = 0; y < h; y += step) {
      var x;
      if (y < band || y >= h - band) { for (x = 0; x < w; x += step) take(x, y); continue; }
      for (x = 0; x < band; x++) take(x, y);
      for (x = Math.max(band, w - band); x < w; x++) take(x, y);
    }
    return { rgb: rgb, clear: clear, n: n };
  }

  // The commonest colours along the edge, most common first: each is the
  // mean of the samples near the busiest 16-level colour bin not yet taken.
  // share is the part of the edge it covers and spread how tightly its
  // samples sit round it (a painted backdrop is nearly uniform; a dark
  // painted scene is not, however dark).
  function modes(s, k) {
    var n = s.length / 3, taken = new Uint8Array(n), out = [], i;
    for (var m = 0; m < k; m++) {
      var bins = new Uint32Array(4096), top = -1, best = 0;
      for (i = 0; i < n; i++) {
        if (taken[i]) continue;
        var bi = (s[i * 3] >> 4) << 8 | (s[i * 3 + 1] >> 4) << 4 | s[i * 3 + 2] >> 4;
        if (++bins[bi] > best) { best = bins[bi]; top = bi; }
      }
      if (top < 0) break;
      var c = [(top >> 8) * 16 + 8, ((top >> 4) & 15) * 16 + 8, (top & 15) * 16 + 8], cnt = 0, spread = 0;
      for (var it = 0; it < 3; it++) {
        var sum = [0, 0, 0];
        cnt = 0; spread = 0;
        for (i = 0; i < n; i++) {
          if (taken[i]) continue;
          var v = dist(s[i * 3], s[i * 3 + 1], s[i * 3 + 2], c);
          if (v > FIT) continue;
          cnt++; spread += v; sum[0] += s[i * 3]; sum[1] += s[i * 3 + 1]; sum[2] += s[i * 3 + 2];
        }
        if (!cnt) break;
        c = [sum[0] / cnt, sum[1] / cnt, sum[2] / cnt];
      }
      for (i = 0; i < n; i++) if (!taken[i] && dist(s[i * 3], s[i * 3 + 1], s[i * 3 + 2], c) <= FIT) taken[i] = 1;
      out.push({ c: c, share: cnt / n, spread: cnt ? spread / cnt : 99 });
    }
    return out;
  }

  // A checkerboard alternates along each edge in runs of one length; a
  // picture whose edge is half sky and half ground does not. Returns the
  // squares' grid (size and offset, from the top and left edges) so a gap
  // inside the figure can be checked against it, or null.
  function checkerGrid(px, cols) {
    var w = px.width, h = px.height, d = px.data, flips = 0, labelled = 0;
    function label(x, y) {
      var i = (y * w + x) * 4;
      if (d[i + 3] < 200) return -1;
      var d0 = dist(d[i], d[i + 1], d[i + 2], cols[0]), d1 = dist(d[i], d[i + 1], d[i + 2], cols[1]);
      return Math.min(d0, d1) > FIT ? -1 : d0 <= d1 ? 0 : 1;
    }
    // Where along one edge the colour changes.
    function line(x0, y0, dx, dy, len) {
      var prev = -1, at = [];
      for (var k = 0; k < len; k++) {
        var c = label(x0 + dx * k, y0 + dy * k);
        if (c < 0) continue;
        labelled++;
        if (prev >= 0 && c !== prev) { flips++; at.push(k); }
        prev = c;
      }
      return at;
    }
    function median(a) { a = a.slice().sort(function (p, q) { return p - q; }); return a.length ? a[a.length >> 1] : 0; }
    var e = Math.max(1, Math.round(Math.min(w, h) * 0.006));
    var top = line(0, e, 1, 0, w), left = line(e, 0, 0, 1, h);
    line(0, h - 1 - e, 1, 0, w); line(w - 1 - e, 0, 0, 1, h);
    if (flips < 8 || labelled / (flips + 1) < 3) return null;
    var gaps = [];
    [top, left].forEach(function (at) { for (var k = 1; k < at.length; k++) gaps.push(at[k] - at[k - 1]); });
    var size = median(gaps.filter(function (g) { return g >= 3; }));
    if (!size) return null;
    function offset(at) { return median(at.map(function (t) { return t % size; })); }
    return { size: size, ox: top.length ? offset(top) : 0, oy: left.length ? offset(left) : 0 };
  }

  // Each pixel's distance to the nearest backdrop colour, and which colour,
  // worked out once per picture and kept on the result.
  function near(px, bg) {
    if (bg._for === px) return bg;
    var N = px.width * px.height, d = px.data, D = new Uint16Array(N), I = new Uint8Array(N), cs = bg.colours;
    for (var i = 0; i < N; i++) {
      var r = d[i * 4], g = d[i * 4 + 1], b = d[i * 4 + 2], best = 65535, bi = 0;
      for (var c = 0; c < cs.length; c++) { var v = dist(r, g, b, cs[c]); if (v < best) { best = v; bi = c; } }
      D[i] = Math.min(65535, Math.round(best)); I[i] = bi;
    }
    bg._for = px; bg._d = D; bg._n = I; bg._m = {};
    return bg;
  }

  // What, if anything, sits behind the subject. kind is 'checker', 'flat',
  // 'clear' (already see-through) or 'busy'; sure is whether to offer it
  // unasked. colours are the backdrop's, a best guess even when not sure, so
  // a person can still clear it by hand.
  function detect(px) {
    var s = border(px), out = { kind: 'busy', sure: false, colours: [] };
    if (!s.n) return out;
    if (s.clear > s.n * 0.2) { out.kind = 'clear'; return out; }
    var m = modes(s.rgb, 2), a = m[0], b = m[1];
    if (!a) return out;
    if (b && dist(a.c[0], a.c[1], a.c[2], b.c) < 18) { a.share += b.share; b = null; }
    if (b && b.share >= 0.12 && a.share + b.share >= 0.6 && (out.grid = checkerGrid(px, [a.c, b.c]))) {
      out.kind = 'checker'; out.colours = [a.c, b.c]; out.sure = true;
    } else {
      out.colours = [a.c];
      out.kind = a.share >= 0.6 ? 'flat' : 'busy';
      out.sure = out.kind === 'flat';
    }
    if (out.sure) out.sure = convincing(px, out);
    return out;
  }

  // Whether clearing at Medium leaves a backdrop's cut, not a scene's. It
  // must clear some but not nearly all of the picture (a figure touching the
  // edges all round, a full-bleed sky), and where it stops the colour must
  // jump: most kept pixels at the cut, or two pixels in, are far from the
  // backdrop. A dark painted scene fades into its shadows, so its cut is soft
  // and it is not offered.
  function convincing(px, bg) {
    var a = mask(px, bg, 'medium'), D = bg._d, w = px.width, N = a.length, tol = TOL.medium, cleared = 0, rim = 0, sharp = 0;
    for (var i = 0; i < N; i++) {
      if (a[i] === 0) { cleared++; continue; }
      var x = i % w;
      if (!((x > 0 && a[i - 1] === 0) || (x < w - 1 && a[i + 1] === 0) || (i >= w && a[i - w] === 0) || (i < N - w && a[i + w] === 0))) continue;
      var m = D[i];
      if (x > 1 && a[i - 2]) m = Math.max(m, D[i - 2]);
      if (x < w - 2 && a[i + 2]) m = Math.max(m, D[i + 2]);
      if (i >= 2 * w && a[i - 2 * w]) m = Math.max(m, D[i - 2 * w]);
      if (i < N - 2 * w && a[i + 2 * w]) m = Math.max(m, D[i + 2 * w]);
      rim++;
      if (m >= 2 * tol) sharp++;
    }
    var f = cleared / N;
    return f >= 0.04 && f <= 0.96 && rim > 0 && sharp / rim >= 0.6;
  }

  // Clears what is joined to the picture's edge and near a backdrop colour,
  // then softens the cut by one pixel so it is not jagged. A painted
  // checkerboard also shows through gaps (between an arm and the body), so
  // enclosed patches that hold both of its colours are cleared too; Strong
  // also clears enclosed patches of a flat backdrop.
  function mask(px, bg, strength) {
    var tol = TOL[strength] || TOL.medium, w = px.width, h = px.height, N = w * h, A = px.data;
    if (!bg.colours.length) bg = { kind: bg.kind, colours: [[255, 255, 255]] };
    near(px, bg);
    if (bg._m[strength]) return bg._m[strength];
    var D = bg._d, I = bg._n, cut = new Uint8Array(N), stack = new Int32Array(N), sp = 0, i, x, y;
    function ok(j) { return D[j] <= tol || A[j * 4 + 3] < 16; }
    function seed(j) { if (!cut[j] && ok(j)) { cut[j] = 1; stack[sp++] = j; } }
    for (x = 0; x < w; x++) { seed(x); seed((h - 1) * w + x); }
    for (y = 0; y < h; y++) { seed(y * w); seed(y * w + w - 1); }
    function flood() {
      while (sp) {
        var j = stack[--sp], jx = j % w;
        if (jx > 0) seed(j - 1);
        if (jx < w - 1) seed(j + 1);
        if (j >= w) seed(j - w);
        if (j < N - w) seed(j + w);
      }
    }
    flood();
    var G = bg.kind === 'checker' ? bg.grid : null;
    var holes = strength !== 'light' && (G || (bg.kind === 'flat' && strength === 'strong'));
    // Whether a gap's pixels sit on the checkerboard's own squares: each one
    // the colour its square should be, away from the squares' blurred edges.
    function onGrid(list, n) {
      var same = 0, scored = 0, z = G.size;
      for (var t = 0; t < n; t++) {
        var j = list[t], gx = (j % w - G.ox + z * 64) % z, gy = ((j / w | 0) - G.oy + z * 64) % z;
        if (gx < 1 || gx > z - 2 || gy < 1 || gy > z - 2) continue;
        scored++;
        if (((Math.floor((j % w - G.ox) / z) + Math.floor(((j / w | 0) - G.oy) / z)) & 1) === I[j]) same++;
      }
      return scored >= 16 && Math.max(same, scored - same) >= scored * 0.85;
    }
    if (holes) {
      var seen = new Uint8Array(N), list = new Int32Array(N), min = Math.max(24, N * (strength === 'strong' ? 0.0002 : 0.0004));
      for (i = 0; i < N; i++) {
        if (cut[i] || seen[i] || !ok(i)) continue;
        var n = 0, head = 0;
        seen[i] = 1; list[n++] = i;
        while (head < n) {
          var j = list[head++], jx = j % w;
          var nb = [jx > 0 ? j - 1 : -1, jx < w - 1 ? j + 1 : -1, j >= w ? j - w : -1, j < N - w ? j + w : -1];
          for (var k = 0; k < 4; k++) { var q = nb[k]; if (q >= 0 && !seen[q] && !cut[q] && ok(q)) { seen[q] = 1; list[n++] = q; } }
        }
        if (n >= min && (G ? onGrid(list, n) : true)) for (var t = 0; t < n; t++) cut[list[t]] = 1;
      }
    }
    var out = new Uint8Array(N);
    for (i = 0; i < N; i++) {
      if (cut[i]) continue;
      x = i % w;
      var edge = (x > 0 && cut[i - 1]) || (x < w - 1 && cut[i + 1]) || (i >= w && cut[i - w]) || (i < N - w && cut[i + w]);
      if (!edge) { out[i] = 255; continue; }
      // A painted edge pixel is part backdrop, part figure: how far it is
      // from the backdrop, against the figure just inside, is how much
      // figure it holds.
      var y0 = (i / w) | 0, f = D[i];
      for (var dy = -2; dy <= 2; dy++) for (var dx = -2; dx <= 2; dx++) {
        var xx = x + dx, yy = y0 + dy;
        if (xx < 0 || yy < 0 || xx >= w || yy >= h) continue;
        var q = yy * w + xx;
        if (!cut[q] && D[q] > f) f = D[q];
      }
      out[i] = Math.round(255 * Math.min(1, D[i] / f));
    }
    bg._m[strength] = out;
    return out;
  }

  // Writes the token maker's picture into out (an ImageData of the same
  // size) for the rectangle x0,y0 to x1,y1 (end exclusive). auto is mask()'s
  // result or null; paint holds the brushes, 1 erase and 2 restore. A
  // half-cleared edge pixel has the backdrop colour taken back out of it, so
  // no light fringe is left round the figure.
  function compose(px, bg, auto, paint, out, x0, y0, x1, y1) {
    var w = px.width, s = px.data, o = out.data, cs = bg && bg.colours, I = bg && bg._for === px ? bg._n : null;
    for (var y = y0; y < y1; y++) {
      for (var x = x0; x < x1; x++) {
        var i = y * w + x, p = i * 4, sa = s[p + 3], pv = paint ? paint[i] : 0;
        var a = pv === 1 ? 0 : pv === 2 || !auto ? sa : Math.min(sa, auto[i]);
        var r = s[p], g = s[p + 1], b = s[p + 2];
        if (a > 0 && a < sa && pv !== 2 && cs && I) {
          var f = a / 255, c = cs[I[i]];
          r = (r - (1 - f) * c[0]) / f; g = (g - (1 - f) * c[1]) / f; b = (b - (1 - f) * c[2]) / f;
        }
        o[p] = r; o[p + 1] = g; o[p + 2] = b; o[p + 3] = a;
      }
    }
  }

  var api = { detect: detect, mask: mask, compose: compose, strengths: ['light', 'medium', 'strong'] };
  if (typeof module === 'object' && module.exports) module.exports = api;
  else root.ChronicleTokenCutout = api;
})(typeof window !== 'undefined' ? window : this);
