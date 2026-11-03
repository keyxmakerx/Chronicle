/**
 * craft_paint.js -- the painted-bench toolkit (CraftPaint)
 *
 * Painted benches (the forge first) are canvas scenes drawn at one fixed
 * logical size, 640x360, and scaled to the screen. This file is what every
 * such scene shares: seeded noise, textures baked once (stone, wood, flags,
 * plaster, slate), light (pools, several lights, glows, contact shadows),
 * film grain and vignette, a particle list, and the baked flipbook fire that
 * every effect level draws. Scenes register with define(); CraftStage
 * (craft_paint_stage.js) runs them. craft_paint.ai.md is the guide.
 */
window.CraftPaint = window.CraftPaint || (function () {
  'use strict';
  var W = 640, H = 360;

  // Seeded, so a baked texture looks the same on every load and every device.
  function rng(seed) {
    var s = seed >>> 0 || 1;
    return function () { s ^= s << 13; s ^= s >>> 17; s ^= s << 5; return ((s >>> 0) % 100000) / 100000; };
  }
  function makeNoise(seed) {
    var r = rng(seed), p = new Float32Array(256 * 256);
    for (var i = 0; i < p.length; i++) p[i] = r();
    function v(x, y) {
      var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi;
      var a = p[((yi & 255) << 8) | (xi & 255)], b = p[((yi & 255) << 8) | ((xi + 1) & 255)],
        c = p[(((yi + 1) & 255) << 8) | (xi & 255)], d = p[(((yi + 1) & 255) << 8) | ((xi + 1) & 255)];
      var u = xf * xf * (3 - 2 * xf), w = yf * yf * (3 - 2 * yf);
      return a + (b - a) * u + (c - a) * w + (a - b - c + d) * u * w;
    }
    function fbm(x, y, o) { var t = 0, amp = .5, f = 1; for (var k = 0; k < (o || 4); k++) { t += v(x * f, y * f) * amp; f *= 2; amp *= .5; } return t; }
    return { v: v, fbm: fbm };
  }
  var N = makeNoise(7);
  function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
  function lerp(a, b, t) { return a + (b - a) * t; }
  function mixc(a, b, t) { return [lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)]; }
  function canvas(w, h) { var c = document.createElement('canvas'); c.width = w; c.height = h; return c; }

  // A texture baked once, pixel by pixel: fn(x, y) gives [r, g, b] or [r, g, b, a].
  function bake(w, h, fn) {
    var c = canvas(w, h), g = c.getContext('2d'), im = g.createImageData(w, h), d = im.data;
    for (var y = 0; y < h; y++) for (var x = 0; x < w; x++) {
      var o = (y * w + x) * 4, col = fn(x, y);
      d[o] = clamp(col[0], 0, 255); d[o + 1] = clamp(col[1], 0, 255); d[o + 2] = clamp(col[2], 0, 255); d[o + 3] = col[3] == null ? 255 : col[3];
    }
    g.putImageData(im, 0, 0); return c;
  }
  // Shared by every scene on the page, so a second forge costs no second bake.
  var cache = {};
  function tex(name, w, h, fn) { var k = name + w + 'x' + h; return cache[k] || (cache[k] = bake(w, h, fn)); }

  // Planks running across: grain, knots, seams and a worn lighter middle.
  function wood(w, h, base, dark) {
    return tex('wood' + base, w, h, function (x, y) {
      var plank = Math.floor(y / 26), py = y % 26, seam = py < 1.5 ? .45 : py > 24.5 ? .7 : 1;
      var g = N.fbm(x * .012 + plank * 9, y * .25 + plank * 3, 3), ring = Math.sin((x * .03 + g * 9 + plank * 2) * 3.1) * .5 + .5;
      var fine = N.v(x * .9, y * .08 + plank * 40) * .25, wear = 1 - Math.abs(py - 13) / 40;
      var t = clamp(ring * .45 + g * .4 + fine, 0, 1), c = mixc(dark, base, t * wear);
      return [c[0] * seam, c[1] * seam, c[2] * seam];
    });
  }
  // Rough stone: rows and blocks of uneven size, chipped edges, a tone per block.
  function stone(w, h, base) {
    var r = rng(5), rows = [], y = 0;
    while (y < h) {
      var rh = 26 + Math.floor(r() * 16), blocks = [], x = -Math.floor(r() * 40);
      while (x < w) { var bw = 38 + Math.floor(r() * 56); blocks.push([x, bw, r()]); x += bw; }
      rows.push([y, rh, blocks]); y += rh;
    }
    return tex('stone' + base, w, h, function (px, py) {
      var row = rows[0], i, j;
      for (i = 0; i < rows.length; i++) if (py >= rows[i][0] && py < rows[i][0] + rows[i][1]) { row = rows[i]; break; }
      var blk = row[2][0];
      for (j = 0; j < row[2].length; j++) if (px >= row[2][j][0] && px < row[2][j][0] + row[2][j][1]) { blk = row[2][j]; break; }
      var ex = Math.min(px - blk[0], blk[0] + blk[1] - px), ey = Math.min(py - row[0], row[0] + row[1] - py);
      var chip = N.fbm(px * .15, py * .15, 2) * 4, e = Math.min(ex, ey) - chip;
      if (e < 1.2) return [base[0] * .32, base[1] * .3, base[2] * .3];
      var n = N.fbm(px * .045, py * .045, 5), fine = N.v(px * .7, py * .7) * .1, pit = N.v(px * .35, py * .35) > .82 ? -.18 : 0;
      var k = .5 + n * .6 + fine + pit + (blk[2] - .5) * .45, bevel = .62 + .38 * Math.min(1, e / 12);
      var top = ey === py - row[0] && ey < 6 ? 1.12 : 1; // top edges catch a little more light
      k *= bevel * top;
      return [base[0] * k, base[1] * k * .97, base[2] * k * .95];
    });
  }
  // Flagstones seen from standing height, receding to the back wall.
  function flags(w, h, top, base) {
    return tex('flags' + base + top, w, h, function (px, py) {
      var yy = py + top - 190, Z = 7000 / yy, X = (px - 320) * Z / 100;
      var gx = ((X % 70) + 70) % 70, gz = ((Z + (Math.floor(X / 70) % 2) * 20) % 55 + 55) % 55, mw = 2.2;
      var fog = Math.min(1, (yy - 70) / 120) * .6 + .4;
      if (gx < mw || gz < mw * 1.4) return [base[0] * .3 * fog, base[1] * .28 * fog, base[2] * .26 * fog];
      var cell = N.v(Math.floor(X / 70) * 3.3, Math.floor(Z / 55) * 2.7), n = N.fbm(X * .05, Z * .05, 4);
      var k = (.6 + n * .5 + (cell - .5) * .3) * fog;
      return [base[0] * k, base[1] * k, base[2] * k];
    });
  }
  function plaster(w, h, base) {
    return tex('plaster' + base, w, h, function (x, y) {
      var n = N.fbm(x * .02, y * .02, 5), f = N.v(x * .8, y * .8) * .08, st = N.fbm(x * .004, y * .01, 2);
      var k = .78 + n * .35 + f - st * .15;
      return [base[0] * k, base[1] * k, base[2] * k];
    });
  }
  function slate(w, h) {
    return tex('slate', w, h, function (x, y) {
      var n = N.fbm(x * .01, y * .03, 4), f = N.v(x * .7, y * .7) * .06, k = .7 + n * .4 + f;
      return [34 * k, 38 * k, 46 * k];
    });
  }

  function rgb(c) { return 'rgb(' + c.map(Math.round).join(',') + ')'; }
  // One light: a pool that darkens everything away from its source, multiplied over the frame.
  function lightPool(g, x, y, r, col, amb) {
    g.save(); g.globalCompositeOperation = 'multiply';
    var gr = g.createRadialGradient(x, y, 0, x, y, r);
    gr.addColorStop(0, rgb(col)); gr.addColorStop(.5, rgb(mixc(col, amb, .55))); gr.addColorStop(1, rgb(amb));
    g.fillStyle = gr; g.fillRect(0, 0, W, H); g.restore();
  }
  // Several lights: an ambient base with each light added, worked out at half size, then multiplied.
  var lm = null;
  function lights(g, list, amb) {
    if (!lm) lm = canvas(W / 2, H / 2);
    var c = lm.getContext('2d');
    c.setTransform(.5, 0, 0, .5, 0, 0); c.globalCompositeOperation = 'source-over';
    c.fillStyle = rgb(amb); c.fillRect(0, 0, W, H); c.globalCompositeOperation = 'lighter';
    list.forEach(function (L) {
      var gr = c.createRadialGradient(L.x, L.y, 0, L.x, L.y, L.r), s = L.col.map(Math.round).join(',');
      gr.addColorStop(0, 'rgb(' + s + ')'); gr.addColorStop(.45, 'rgba(' + s + ',.45)'); gr.addColorStop(1, 'rgba(' + s + ',0)');
      c.fillStyle = gr; c.fillRect(0, 0, W, H);
    });
    g.save(); g.globalCompositeOperation = 'multiply'; g.drawImage(lm, 0, 0, W, H); g.restore();
  }
  // The glow round anything bright, added in light.
  function glow(g, x, y, r, col, a) {
    g.save(); g.globalCompositeOperation = 'lighter';
    var gr = g.createRadialGradient(x, y, 0, x, y, r), s = col.join(',');
    gr.addColorStop(0, 'rgba(' + s + ',' + a + ')'); gr.addColorStop(1, 'rgba(' + s + ',0)');
    g.fillStyle = gr; g.beginPath(); g.arc(x, y, r, 0, 7); g.fill(); g.restore();
  }
  // A soft contact shadow, pushed away from the light at lx.
  function shadow(g, x, y, rx, ry, lx, a) {
    var dx = clamp((x - lx) * .08, -18, 18);
    g.save(); g.translate(x + dx, y); g.scale(1, ry / rx);
    var gr = g.createRadialGradient(0, 0, 0, 0, 0, rx);
    gr.addColorStop(0, 'rgba(0,0,0,' + (a || .55) + ')'); gr.addColorStop(1, 'rgba(0,0,0,0)');
    g.fillStyle = gr; g.beginPath(); g.arc(0, 0, rx, 0, 7); g.fill(); g.restore();
  }
  // Fine grain, so flat gradients read as a surface rather than banding.
  var grainTex = null;
  function grain(g, a) {
    if (!grainTex) { var r = rng(3); grainTex = bake(160, 160, function () { var v = r() * 255; return [v, v, v, 255]; }); }
    g.save(); g.globalAlpha = a || .05; g.globalCompositeOperation = 'overlay';
    g.fillStyle = g.createPattern(grainTex, 'repeat'); g.fillRect(0, 0, W, H); g.restore();
  }
  function vignette(g, a) {
    var gr = g.createRadialGradient(W / 2, H / 2, H * .35, W / 2, H / 2, W * .65);
    gr.addColorStop(0, 'rgba(0,0,0,0)'); gr.addColorStop(1, 'rgba(0,0,0,' + (a || .55) + ')');
    g.fillStyle = gr; g.fillRect(0, 0, W, H);
  }

  // Particles: x, y, vx, vy, life and max, plus whatever a scene draws them by.
  // g is gravity, drag a per-frame damping at 60 fps, wob a sideways wobble,
  // and bounce what is left of the fall when one meets floorY.
  function Particles() { this.list = []; }
  Particles.prototype.add = function (p) { p.life = p.max = p.max || 1; this.list.push(p); };
  Particles.prototype.step = function (dt, floorY) {
    for (var i = this.list.length - 1; i >= 0; i--) {
      var p = this.list[i], d = p.drag ? Math.pow(p.drag, dt * 60) : 1;
      p.vy += (p.g || 0) * dt; p.vx *= d; p.vy *= d;
      if (p.wob) p.vx += Math.sin(p.t0 + p.life * p.wob) * 8 * dt;
      p.x += p.vx * dt; p.y += p.vy * dt; p.life -= dt;
      if (p.bounce && floorY && p.y > floorY && p.vy > 0) { p.y = floorY; p.vy *= -p.bounce; p.vx *= .55; }
      if (p.life <= 0) this.list.splice(i, 1);
    }
  };

  /* The fire every level draws: 24 frames of rising noise painted once into
     one picture (a flipbook) and played as sprites. Painting flames as
     particles costs every frame; the flipbook costs a copy per flame, so the
     lowest level still has real fire. */
  var book = null;
  function fireBook() {
    if (book) return book;
    var F = 24, cw = 96, ch = 128, atlas = canvas(cw * 6, ch * 4), ag = atlas.getContext('2d');
    var im = ag.createImageData(cw * 6, ch * 4), d = im.data;
    for (var f = 0; f < F; f++) {
      var ph = f / F, ox = (f % 6) * cw, oy = Math.floor(f / 6) * ch;
      for (var y = 0; y < ch; y++) for (var x = 0; x < cw; x++) {
        var u = x / cw, yb = 1 - y / ch, cx = Math.abs(u - .5) * 2;
        // Two copies of the rising noise, crossfaded, so the last frame flows back into the first.
        var n1 = N.fbm(u * 4 + 3, -(yb - ph) * 3.2 + 9, 4), n2 = N.fbm(u * 4 + 3, -(yb - ph + 1) * 3.2 + 9, 4), n = n1 * (1 - ph) + n2 * ph;
        var width = .95 * Math.pow(1 - yb, .55) + .05, shape = Math.max(0, 1 - cx / width);
        var I = Math.min(1, Math.max(0, n * 1.9 * Math.pow(shape, .8) * Math.pow(1 - yb, .9) - yb * .35 - .12) * 1.5);
        var o = ((oy + y) * cw * 6 + ox + x) * 4;
        d[o] = Math.min(255, I * 3 * 255); d[o + 1] = Math.min(255, Math.max(0, I * 3 - 1) * 255) * .85;
        d[o + 2] = Math.min(255, Math.max(0, I * 3 - 2) * 255) * .6; d[o + 3] = Math.min(255, I * 400);
      }
    }
    ag.putImageData(im, 0, 0);
    book = { atlas: atlas, F: F, cw: cw, ch: ch };
    return book;
  }
  // spots: [x, phase, flip (1 or -1), width, height, alpha] per flame, standing on the line at y.
  function drawFire(g, t, y, spots) {
    var fb = fireBook();
    g.save(); g.globalCompositeOperation = 'lighter';
    spots.forEach(function (s) {
      var f = Math.floor(t * 20 + s[1]) % fb.F, sx = (f % 6) * fb.cw, sy = Math.floor(f / 6) * fb.ch;
      g.save(); g.translate(s[0], y); g.scale(s[2], 1); g.globalAlpha = s[5];
      g.drawImage(fb.atlas, sx, sy, fb.cw, fb.ch, -s[3] / 2, -s[4], s[3], s[4]); g.restore();
    });
    g.restore();
  }

  // Scenes: define(name, make) where make(g, canvas) returns the scene (see craft_paint.ai.md).
  var scenes = {};
  function define(name, make) { scenes[name] = make; }

  return { W: W, H: H, N: N, rng: rng, noise: makeNoise, clamp: clamp, lerp: lerp, mixc: mixc, canvas: canvas, bake: bake, tex: tex,
    wood: wood, stone: stone, flags: flags, plaster: plaster, slate: slate, lights: lights, lightPool: lightPool, glow: glow,
    shadow: shadow, grain: grain, vignette: vignette, Particles: Particles, fireBook: fireBook, drawFire: drawFire,
    scenes: scenes, define: define };
})();
