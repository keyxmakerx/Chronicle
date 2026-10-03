/**
 * calendar_era_blend.js — the era colour field behind the calendar's days.
 *
 * Each era tints the day cells faintly with its two colours; only where one
 * era ends and the next begins do the colours strengthen and curl into each
 * other, in the entering era's style (gas mixing or ink in water). The
 * calendar's era look (colours on/off, feel, intensity, speed) and each
 * era's own feel scale it.
 *
 * Motion is a courtesy, never a cost: a weak device gets a coarser, cheaper
 * texture drawn at most ~30 times a second; prefers-reduced-motion gets a
 * still wash; and the motion eases to a standstill (then the loop stops
 * entirely) when the tab is hidden, the window loses focus, the pointer
 * leaves, or nothing has been touched for a while. Any input eases it back.
 * When nothing should move, it draws once and stops.
 *
 * calendar_view.js mounts one painter per calendar (Chronicle.calendarEra
 * Blend.create); the structure editor's Era look preview mounts another.
 * The pure helpers are exported for test/js/calendar_era_blend.test.mjs.
 */
(function () {
  'use strict';

  var PRESETS = {
    still: { label: 'Still', intensity: 1, speed: 0, hint: 'The colours show but never move.' },
    subtle: { label: 'Slow and subtle', intensity: 1, speed: 1, hint: 'A faint tint that drifts slowly, with a gentle swirl where eras meet. The default.' },
    lively: { label: 'Lively', intensity: 1.7, speed: 1.9, hint: 'Stronger colours and a quicker swirl where eras meet.' }
  };
  var PRESET_KEYS = ['still', 'subtle', 'lively'];
  var STYLES = [['gas', 'Gas mixing'], ['ink', 'Ink in water']];
  var PALETTES = [
    ['Ember', '#6e1a2a', '#d6893a'], ['Tide', '#1d3f6e', '#5aa6a0'], ['Moss', '#2f4a2a', '#a3b55a'],
    ['Dusk', '#3b2a5e', '#c07aa8'], ['Ash', '#3a3a40', '#c46a3a'], ['Gilt', '#5a3c12', '#e0b450']
  ];
  // The fine-tune sliders' range; the server refuses anything outside it.
  var LIMITS = { intensity: [0.5, 3], speed: [0, 2.5] };
  var DEFAULT_LOOK = { colors_on: true, feel: 'subtle', intensity: 1, speed: 1 };

  var IDLE_MS = 8000;      // no input for this long eases the motion to rest
  var UNIT_PX = 180;       // texture scale: one noise unit per ~two day cells
  var SEAM_HALF = 0.45;    // seam half-width, in day cells
  var SAT = 0.62;          // how much of each era colour's saturation is kept

  function clamp01(x) { return x < 0 ? 0 : x > 1 ? 1 : x; }
  function sstep(a, b, x) { var t = clamp01((x - a) / (b - a)); return t * t * (3 - 2 * t); }

  // hexRGB reads #rgb or #rrggbb; anything else is a neutral grey, so a bad
  // stored colour can never break the drawing.
  function hexRGB(h) {
    var s = String(h || '').trim();
    if (/^#[0-9a-f]{3}$/i.test(s)) s = '#' + s[1] + s[1] + s[2] + s[2] + s[3] + s[3];
    if (!/^#[0-9a-f]{6}$/i.test(s)) return [128, 128, 128];
    return [parseInt(s.slice(1, 3), 16), parseInt(s.slice(3, 5), 16), parseInt(s.slice(5, 7), 16)];
  }
  // soften pulls a colour toward its own grey so the tint never shouts.
  function soften(h) {
    var c = hexRGB(h), l = c[0] * 0.3 + c[1] * 0.59 + c[2] * 0.11;
    return c.map(function (x) { return l + (x - l) * SAT; });
  }

  // The calendar's look, with any missing or out-of-range part replaced by
  // the default, so a hand-edited response can't stall or blow out the field.
  function normalizeLook(l) {
    l = l || {};
    var feel = l.feel === 'custom' || PRESETS[l.feel] ? l.feel : DEFAULT_LOOK.feel;
    function num(v, lim, d) { v = +v; return isFinite(v) ? Math.max(lim[0], Math.min(lim[1], v)) : d; }
    return {
      colors_on: l.colors_on !== false,
      feel: feel,
      intensity: num(l.intensity, LIMITS.intensity, 1),
      speed: num(l.speed, LIMITS.speed, 1)
    };
  }
  // An era's own feel (a preset) wins; otherwise the calendar's.
  function feelOf(look, era) {
    if (era && era.feel && PRESETS[era.feel]) return { intensity: PRESETS[era.feel].intensity, speed: PRESETS[era.feel].speed };
    look = normalizeLook(look);
    if (PRESETS[look.feel]) return { intensity: PRESETS[look.feel].intensity, speed: PRESETS[look.feel].speed };
    return { intensity: look.intensity, speed: look.speed };
  }
  function matchPreset(intensity, speed) {
    for (var i = 0; i < PRESET_KEYS.length; i++) {
      var p = PRESETS[PRESET_KEYS[i]];
      if (Math.abs(p.intensity - intensity) < 0.05 && Math.abs(p.speed - speed) < 0.05) return PRESET_KEYS[i];
    }
    return 'custom';
  }
  function wordI(v) { return v < 0.8 ? 'faint' : v < 1.35 ? 'subtle' : v < 2.1 ? 'clear' : 'strong'; }
  function wordS(v) { return v < 0.05 ? 'still' : v < 0.75 ? 'slower' : v < 1.35 ? 'slow' : v < 2.05 ? 'lively' : 'quick'; }

  /* ---------- gradient noise ---------- */
  var perm = new Uint8Array(512);
  (function () {
    var s = 1337, p = [], i;
    for (i = 0; i < 256; i++) p.push(i);
    for (i = 255; i > 0; i--) { s = (s * 16807) % 2147483647; var j = s % (i + 1), t = p[i]; p[i] = p[j]; p[j] = t; }
    for (i = 0; i < 512; i++) perm[i] = p[i & 255];
  })();
  var GX = [0.7071, -0.7071, 0.7071, -0.7071, 1, -1, 0, 0], GY = [0.7071, 0.7071, -0.7071, -0.7071, 0, 0, 1, -1];
  function noise(x, y) {
    var fx = Math.floor(x), fy = Math.floor(y), xf = x - fx, yf = y - fy, xi = fx & 255, yi = fy & 255;
    var u = xf * xf * xf * (xf * (xf * 6 - 15) + 10), v = yf * yf * yf * (yf * (yf * 6 - 15) + 10);
    var a = perm[xi] + yi, b = perm[xi + 1] + yi;
    var g00 = perm[a] & 7, g01 = perm[a + 1] & 7, g10 = perm[b] & 7, g11 = perm[b + 1] & 7;
    var n00 = GX[g00] * xf + GY[g00] * yf, n10 = GX[g10] * (xf - 1) + GY[g10] * yf;
    var n01 = GX[g01] * xf + GY[g01] * (yf - 1), n11 = GX[g11] * (xf - 1) + GY[g11] * (yf - 1);
    var x1 = n00 + u * (n10 - n00), x2 = n01 + u * (n11 - n01);
    return (x1 + v * (x2 - x1)) * 1.4;
  }
  function fbm(x, y, o) {
    var s = 0, a = 0.5;
    for (var i = 0; i < o; i++) { s += a * noise(x, y); x = x * 2.03 + 17.1; y = y * 2.03 + 3.7; a *= 0.5; }
    return s;
  }

  /* ---------- per-pixel era textures ---------- */
  // Each writes a colour into out[0..2]. u,v are page-space units so the
  // texture flows continuously across cells; T is the era's own clock.
  function sample(out, style, pa, pb, u, v, T, o1, o2) {
    var qx, qy, n;
    if (style === 'ink') {
      var px = u * 0.75, py = v * 0.75, t2 = T * 0.6;
      qx = fbm(px + t2 * 0.07, py - t2 * 0.04, o1); qy = fbm(px + 5.2 - t2 * 0.05, py + 1.3 + t2 * 0.03, o1);
      n = fbm(px + 2.4 * qx, py + 2.4 * qy, o2);
      var dens = sstep(0.2, 0.95, clamp01(0.5 + n * 1.3));
      var r = clamp01(1 - Math.abs(n + 0.08) * 4), fil = r * r * r * 0.45;
      var ink = clamp01(dens * 0.85 + fil);
      var wr = pb[0] + (255 - pb[0]) * 0.22, wg = pb[1] + (255 - pb[1]) * 0.22, wb = pb[2] + (255 - pb[2]) * 0.22;
      out[0] = wr + (pa[0] * 0.85 - wr) * ink; out[1] = wg + (pa[1] * 0.85 - wg) * ink; out[2] = wb + (pa[2] * 0.85 - wb) * ink;
      return;
    }
    var gx = u * 0.6, gy = v * 0.6;
    qx = fbm(gx + T * 0.05, gy - T * 0.03, o1); qy = fbm(gx + 5.2 - T * 0.04, gy + 1.3 + T * 0.02, o1);
    var rx = fbm(gx + 1.5 * qx + 1.7 + T * 0.04, gy + 1.5 * qy + 9.2, o1), ry = fbm(gx + 1.5 * qx + 8.3, gy + 1.5 * qy + 2.8 - T * 0.035, o1);
    n = fbm(gx + 1.4 * rx, gy + 1.4 * ry, o2);
    var t = clamp01(n * 1.6 + 0.5), k = sstep(0.05, 0.95, t);
    var lum = 0.9 + 0.24 * (t - 0.5) + 0.18 * qy;
    var cr = (pa[0] + (pb[0] - pa[0]) * k) * lum, cg = (pa[1] + (pb[1] - pa[1]) * k) * lum, cb = (pa[2] + (pb[2] - pa[2]) * k) * lum;
    var wl = 0.1 * sstep(0.6, 1, t);
    out[0] = cr + (255 - cr) * wl; out[1] = cg + (255 - cg) * wl; out[2] = cb + (255 - cb) * wl;
  }
  // Reduced motion: a still, soft two-colour wash per era, no noise.
  function sampleStill(out, pa, pb, u, v) {
    var t = 0.5 + 0.32 * Math.sin(u * 1.3 + v * 0.9) + 0.12 * Math.sin(v * 2.1 - u * 0.6);
    out[0] = pa[0] + (pb[0] - pa[0]) * t; out[1] = pa[1] + (pb[1] - pa[1]) * t; out[2] = pa[2] + (pb[2] - pa[2]) * t;
  }

  /* ---------- device capability ---------- */
  // A device that reports few cores or little memory, or whose first second
  // of frames runs slow, gets the light version.
  function deviceIsLimited(nav) {
    nav = nav || (typeof navigator !== 'undefined' ? navigator : {});
    return !!((nav.hardwareConcurrency && nav.hardwareConcurrency <= 4) || (nav.deviceMemory && nav.deviceMemory <= 4));
  }
  // canvasSize: the drawing is coarse on purpose (it is a soft field), and a
  // pixel cap keeps a very wide calendar from costing more.
  function canvasSize(cw, ch, light) {
    var div = light ? 9 : 4, cap = light ? 2600 : 16000;
    var w = Math.max(8, Math.round(cw / div)), h = Math.max(4, Math.round(ch / div));
    if (w * h > cap) { var s = Math.sqrt(cap / (w * h)); w = Math.max(8, Math.round(w * s)); h = Math.max(4, Math.round(h * s)); }
    return [w, h];
  }

  // daysClip is the outline of the month's real day cells, in box px: the
  // canvas is drawn coarse and scaled up, so its soft edge is cut here to
  // the cells' own edges. Null when the scene has no grid to follow.
  function daysClip(sc) {
    var rows = sc && sc.rows, cols = sc && sc.cols;
    if (!rows || !rows.length || !cols || !sc.days) return null;
    var W = sc.box.width, p = W / cols, first = sc.off || 0, last = first + sc.days - 1;
    var r0 = Math.floor(first / cols), rL = Math.min(rows.length - 1, Math.floor(last / cols));
    if (r0 > rL) return null;
    var c0 = first - r0 * cols, cL = rL === Math.floor(last / cols) ? last % cols : cols - 1;
    var top = function (r) { return rows[r].top; }, bot = function (r) { return rows[r].top + rows[r].height; };
    var x0 = c0 * p, xL = (cL + 1) * p;
    if (r0 === rL) return [[x0, top(r0)], [xL, top(r0)], [xL, bot(r0)], [x0, bot(r0)]];
    return [[x0, top(r0)], [W, top(r0)], [W, bot(rL - 1)], [xL, bot(rL - 1)], [xL, bot(rL)], [0, bot(rL)], [0, bot(r0)], [x0, bot(r0)]];
  }

  /* ---------- the painter ---------- */
  // One shared input watcher wakes every painter; each painter keeps its
  // own ease so a calendar and its settings preview rest independently.
  var painters = [];
  var watching = false;
  function broadcast(fn) { painters.slice().forEach(function (p) { fn(p); }); }
  function watchInput() {
    if (watching || typeof window === 'undefined') return;
    watching = true;
    window.addEventListener('pointermove', function () { broadcast(function (p) { p._poke(); }); }, { passive: true });
    ['pointerdown', 'keydown', 'wheel', 'touchstart'].forEach(function (ev) {
      window.addEventListener(ev, function () { broadcast(function (p) { p.wake(); }); }, { passive: true, capture: true });
    });
    window.addEventListener('mouseout', function (e) { if (!e.relatedTarget) broadcast(function (p) { p.rest(); }); });
    document.addEventListener('visibilitychange', function () { broadcast(function (p) { if (document.hidden) p.rest(); else p.wake(); }); });
    window.addEventListener('blur', function () { broadcast(function (p) { p.rest(); }); });
    if (window.matchMedia) {
      var mq = window.matchMedia('(prefers-reduced-motion: reduce)');
      var onChange = function () { broadcast(function (p) { p.refresh(); }); };
      if (mq.addEventListener) mq.addEventListener('change', onChange); else if (mq.addListener) mq.addListener(onChange);
      var dq = window.matchMedia('(prefers-color-scheme: dark)');
      if (dq.addEventListener) dq.addEventListener('change', onChange); else if (dq.addListener) dq.addListener(onChange);
    }
  }

  // surfaceRGB resolves any CSS colour (oklch included) to RGB by painting
  // it, since the surface the tint mixes into is a theme token.
  var probe = null;
  function surfaceRGB(css) {
    try {
      if (!probe) { probe = document.createElement('canvas'); probe.width = probe.height = 1; }
      var c = probe.getContext('2d', { willReadFrequently: true });
      c.clearRect(0, 0, 1, 1);
      c.fillStyle = '#fff';
      c.fillStyle = css;
      c.fillRect(0, 0, 1, 1);
      var d = c.getImageData(0, 0, 1, 1).data;
      return [d[0], d[1], d[2]];
    } catch (e) { return [255, 255, 255]; }
  }

  function Painter(host, opts) {
    this.host = host;
    this.opts = opts || {};
    this.cv = document.createElement('canvas');
    this.cv.className = 'era-field';
    this.cv.setAttribute('aria-hidden', 'true');
    this.fade = document.createElement('canvas');
    this.fade.className = 'era-field era-fade';
    this.fade.setAttribute('aria-hidden', 'true');
    this.ctx = this.cv.getContext('2d', { willReadFrequently: true });
    this.fctx = this.fade.getContext('2d');
    this.img = null;
    this.scene = null;
    this.look = normalizeLook(null);
    this.eraT = {};        // per-era clocks, so each era keeps its own speed
    this.mixT = 0;         // the seam's swirl clock
    this.activity = 1; this.target = 1;
    this.rafId = 0; this.lastTs = 0; this.frames = 0;
    this.measured = { frames: 0, sum: 0, done: false, slow: false };
    this.limited = deviceIsLimited();
    this.surf = [255, 255, 255]; this.dark = false;
    this.idleTimer = 0;
    this._tick = this._tick.bind(this);
    painters.push(this);
    watchInput();
    this._resetIdle();
  }

  Painter.prototype.reduced = function () {
    if (this.opts.reduced) return this.opts.reduced();
    return typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  };
  Painter.prototype.light = function () { return this.limited || this.measured.slow; };

  // Motion is wanted only when colours are on, the viewer allows motion,
  // and an era on screen has a speed.
  Painter.prototype.canMove = function () {
    var sc = this.scene;
    if (!sc || !sc.eras.length || !this.look.colors_on || this.reduced()) return false;
    var look = this.look;
    return sc.eras.some(function (e) { return feelOf(look, e).speed > 0; });
  };

  // scene: { box:{left,top,width,height} in host px, rows:[{top,height}] in
  // box px, cols, off (column of day 1), days, split (day the second era
  // begins, or null), eras:[era, era?] } where an era is
  // { key, color, color_2, style, feel }.
  Painter.prototype.setScene = function (scene, o) {
    o = o || {};
    if (o.crossfade && this.scene && this.img && this.look.colors_on && !this.reduced()) {
      this.fade.width = this.cv.width; this.fade.height = this.cv.height;
      this.fctx.drawImage(this.cv, 0, 0);
      this.fade.style.transition = 'none'; this.fade.style.opacity = '1';
      this._placeEl(this.fade, this.scene);
      void this.fade.offsetWidth;
      this.fade.style.transition = 'opacity .6s ease'; this.fade.style.opacity = '0';
    }
    this.scene = scene;
    var self = this;
    (scene.eras || []).forEach(function (e, i) { if (self.eraT[e.key] == null) self.eraT[e.key] = 3 + 4 * i; });
    this.refresh();
  };
  Painter.prototype.setLook = function (look) { this.look = normalizeLook(look); this.refresh(); };

  Painter.prototype._placeEl = function (el, sc) {
    var box = sc.box;
    if (el.parentNode !== this.host) this.host.insertBefore(el, this.host.firstChild);
    el.style.left = box.left + 'px'; el.style.top = box.top + 'px';
    el.style.width = box.width + 'px'; el.style.height = box.height + 'px';
    var clip = daysClip(sc);
    el.style.clipPath = clip ? 'polygon(' + clip.map(function (p) { return p[0] + 'px ' + p[1] + 'px'; }).join(',') + ')' : '';
  };

  // refresh re-reads everything that shapes a frame (size, theme, motion
  // permission), draws once, and starts or stops the loop to match.
  Painter.prototype.refresh = function () {
    var sc = this.scene;
    var show = !!(sc && sc.eras.length && this.look.colors_on && sc.box.width > 0);
    this.cv.style.opacity = show ? '' : '0';
    if (!sc) return;
    this._placeEl(this.cv, sc);
    this._placeEl(this.fade, sc);
    var size = canvasSize(sc.box.width, sc.box.height, this.light());
    if (this.cv.width !== size[0] || this.cv.height !== size[1] || !this.img) {
      this.cv.width = size[0]; this.cv.height = size[1];
      this.img = this.ctx.createImageData(size[0], size[1]);
    }
    var bg = this.opts.surface ? this.opts.surface() : '#fff';
    this.surf = surfaceRGB(bg);
    this.dark = this.surf[0] + this.surf[1] + this.surf[2] < 300;
    if (!show) { this._stop(); return; }
    this.draw();
    if (this.canMove()) this._ensureLoop(); else this._stop();
  };

  Painter.prototype.draw = function () {
    var sc = this.scene;
    if (!this.img || !sc || !sc.eras.length || !this.look.colors_on) return;
    var W = this.cv.width, H = this.cv.height, d = this.img.data, out = [0, 0, 0];
    var cw = sc.box.width, ch = sc.box.height, rows = sc.rows, nRows = rows.length, cols = sc.cols, off = sc.off, days = sc.days;
    var pitchX = cw / cols;
    var still = this.reduced(), light = this.light(), o1 = light ? 1 : 2, o2 = light ? 2 : 3;
    var e0 = sc.eras[0], e1 = sc.eras[sc.eras.length - 1];
    var f0 = feelOf(this.look, e0), f1 = feelOf(this.look, e1), I0 = f0.intensity, I1 = f1.intensity;
    var a0 = soften(e0.color), b0 = soften(e0.color_2 || e0.color), a1 = soften(e1.color), b1 = soften(e1.color_2 || e1.color);
    var mean0 = [0, 1, 2].map(function (k) { return (a0[k] + b0[k]) * 0.5; }), mean1 = [0, 1, 2].map(function (k) { return (a1[k] + b1[k]) * 0.5; });
    var T0 = this.eraT[e0.key] || 0, T1 = this.eraT[e1.key] || 0, mixT = this.mixT;
    var split = sc.split != null && sc.eras.length > 1 ? sc.split - 0.5 : null;
    // Tint strength: dark surfaces need a little more to show at all.
    var baseC = this.dark ? 0.17 : 0.11, seamC = this.dark ? 0.34 : 0.24;
    var tend = still ? 0 : 0.55;        // how far tendrils reach across the seam, in cells
    var sharp = still ? 0.55 : 0.08;    // softness of the era interface
    var EDGE_ROWS = 0.2, surf = this.surf;
    var clipped = !!daysClip(sc);
    function alphaOf(s) { return sstep(0.3, 0.5, s) * (1 - sstep(days + 0.5, days + 0.7, s)); }
    var i = 0, row = 0;
    for (var y = 0; y < H; y++) {
      var yc = (y + 0.5) * ch / H;
      while (row < nRows - 1 && yc >= rows[row + 1].top) row++;
      while (row > 0 && yc < rows[row].top) row--;
      var rh = rows[row].height || 1, fy = clamp01((yc - rows[row].top) / rh);
      // Neighbouring rows blend in near a row's edge, so the line between
      // rows has no hard seam where an era wraps onto the next week.
      var nb = fy < 0.5 ? row - 1 : row + 1, dist = fy < 0.5 ? fy : 1 - fy;
      var v = yc / UNIT_PX, hasNb = nb >= 0 && nb < nRows;
      var wnb = hasNb ? 1 - sstep(-EDGE_ROWS, EDGE_ROWS, dist) : 0;
      for (var x = 0; x < W; x++) {
        var xc = (x + 0.5) * cw / W, colF = xc / pitchX, u = xc / UNIT_PX;
        // s runs along the day sequence: a cell's centre is its day number.
        var sA = row * cols + colF - off + 0.5, sB = nb * cols + colF - off + 0.5;
        // With the outline clip (daysClip) every pixel is opaque: a clear
        // pixel next to a day would be smeared into it when the coarse
        // canvas is scaled up. Without one, the pixel's own cell decides.
        var alpha = clipped ? 1 : alphaOf(sA);
        if (alpha <= 0) { d[i] = d[i + 1] = d[i + 2] = d[i + 3] = 0; i += 4; continue; }
        var r, g, b, mr, mg, mb, seam = 0, mix = 0, w = 0;
        if (split === null) {
          if (still) sampleStill(out, a0, b0, u, v); else sample(out, e0.style, a0, b0, u, v, T0, o1, o2);
          r = out[0]; g = out[1]; b = out[2]; mr = mean0[0]; mg = mean0[1]; mb = mean0[2];
        } else {
          var dA = sA - split, dB = sB - split;
          var envA = Math.exp(-(dA * dA) / 0.49), envB = hasNb ? Math.exp(-(dB * dB) / 0.49) : 0;
          var env = envA * (1 - wnb) + envB * wnb;
          // The swirl is only evaluated near the seam, where it does anything.
          if (tend && env > 0.01) {
            var wx = fbm(u * 2.4 + mixT * 0.22, v * 2.4 - mixT * 0.16, light ? 1 : 2), wy = fbm(u * 2.4 + 5.3 - mixT * 0.18, v * 2.4 + 2.1 + mixT * 0.14, light ? 1 : 2);
            // Stretched along the row, so tendrils reach across the seam.
            w = fbm(u * 3.2 + wx * 1.6, v * 9 + wy * 2.4, light ? 2 : 3) * 2.2;
          }
          var mA = sstep(-sharp, sharp, dA + w * tend * envA), mB = sstep(-sharp, sharp, dB + w * tend * envB);
          mix = mA * (1 - wnb) + mB * wnb;
          var sh2 = SEAM_HALF * SEAM_HALF;
          seam = Math.exp(-(dA * dA) / sh2) * (1 - wnb) + (hasNb ? Math.exp(-(dB * dB) / sh2) * wnb : 0);
          // Inside the seam both colours are pulled along the swirl, so they curl into each other.
          var du = env * w * 0.18, dv = env * w * 0.12;
          r = g = b = 0;
          if (mix < 0.999) {
            if (still) sampleStill(out, a0, b0, u, v); else sample(out, e0.style, a0, b0, u + du, v - dv, T0, o1, o2);
            r = out[0]; g = out[1]; b = out[2];
          }
          if (mix > 0.001) {
            if (still) sampleStill(out, a1, b1, u, v); else sample(out, e1.style, a1, b1, u - du, v + dv, T1, o1, o2);
            if (mix >= 0.999) { r = out[0]; g = out[1]; b = out[2]; } else { r += (out[0] - r) * mix; g += (out[1] - g) * mix; b += (out[2] - b) * mix; }
          }
          mr = mean0[0] + (mean1[0] - mean0[0]) * mix; mg = mean0[1] + (mean1[1] - mean0[1]) * mix; mb = mean0[2] + (mean1[2] - mean0[2]) * mix;
        }
        var I = I0 + (I1 - I0) * mix;
        // Flatten the texture toward the era's mean colour; less so in the seam.
        var farC = Math.min(1, 0.3 * I), c = farC + (Math.min(1, 0.9 * I) - farC) * seam;
        r = mr + (r - mr) * c; g = mg + (g - mg) * c; b = mb + (b - mb) * c;
        // Where the colours meet, a faint lift (gas) or shadow (ink) along the interface.
        if (split !== null && !still) {
          var edge = 4 * mix * (1 - mix) * seam;
          if (edge > 0.02) {
            var ek = edge * 0.5;
            if (e1.style === 'ink') { var q = 1 - ek * 0.25; r *= q; g *= q; b *= q; } else { r += (255 - r) * ek * 0.4; g += (255 - g) * ek * 0.4; b += (255 - b) * ek * 0.4; }
          }
        }
        var baseK = Math.min(0.8, baseC * I), k = baseK + (Math.min(0.95, seamC * I) - baseK) * seam;
        d[i] = surf[0] + (r - surf[0]) * k; d[i + 1] = surf[1] + (g - surf[1]) * k; d[i + 2] = surf[2] + (b - surf[2]) * k; d[i + 3] = alpha * 255;
        i += 4;
      }
    }
    this.ctx.putImageData(this.img, 0, 0);
  };

  /* ---------- the loop ---------- */
  Painter.prototype._ensureLoop = function () {
    if (this.rafId || !this.canMove()) return;
    if (this.target <= 0 && this.activity <= 0) return;
    this.lastTs = 0;
    this.rafId = requestAnimationFrame(this._tick);
  };
  Painter.prototype._stop = function () { if (this.rafId) { cancelAnimationFrame(this.rafId); this.rafId = 0; } };
  Painter.prototype._tick = function (ts) {
    this.rafId = 0;
    if (!this.canMove()) { this.draw(); return; }
    var light = this.light();
    if (light && this.lastTs && ts - this.lastTs < 31) { this.rafId = requestAnimationFrame(this._tick); return; }
    var dt = this.lastTs ? Math.min(0.1, (ts - this.lastTs) / 1000) : 1 / 60;
    var m = this.measured;
    if (this.lastTs && !m.done && !document.hidden) {
      m.frames++; m.sum += dt;
      if (m.sum > 1) { m.done = true; if (m.sum / m.frames * 1000 > 25) { m.slow = true; this.refresh(); return; } }
    }
    this.lastTs = ts;
    // One eased clock: about 1 s to start, about 2 s to come to rest.
    if (this.activity < this.target) this.activity = Math.min(this.target, this.activity + dt / 1.0);
    else if (this.activity > this.target) this.activity = Math.max(this.target, this.activity - dt / 2.0);
    var k = this.activity * this.activity * (3 - 2 * this.activity), step = dt * k, look = this.look, self = this, sp = 0;
    this.scene.eras.forEach(function (e) {
      var f = feelOf(look, e);
      self.eraT[e.key] = (self.eraT[e.key] || 0) + step * 0.14 * f.speed;
      if (f.speed > sp) sp = f.speed;
    });
    this.mixT += step * sp;
    this.draw();
    this.frames++;
    if (this.activity <= 0 && this.target <= 0) { this.activity = 0; return; } // at rest: stop drawing
    this.rafId = requestAnimationFrame(this._tick);
  };

  Painter.prototype._resetIdle = function () {
    var self = this;
    clearTimeout(this.idleTimer);
    this.idleTimer = setTimeout(function () { self.rest(); }, IDLE_MS);
  };
  Painter.prototype._poke = function () { if (this.target === 0 || this.activity < 1) this.wake(); else this._resetIdle(); };
  Painter.prototype.wake = function () {
    if (typeof document !== 'undefined' && document.hidden) return;
    this.target = 1; this._resetIdle(); this._ensureLoop();
  };
  Painter.prototype.rest = function () {
    this.target = 0; clearTimeout(this.idleTimer);
    if (this.activity > 0) this._ensureLoop();
  };
  Painter.prototype.destroy = function () {
    this._stop(); clearTimeout(this.idleTimer);
    var i = painters.indexOf(this); if (i >= 0) painters.splice(i, 1);
    if (this.cv.parentNode) this.cv.parentNode.removeChild(this.cv);
    if (this.fade.parentNode) this.fade.parentNode.removeChild(this.fade);
  };
  // What a live check needs to see without reaching into the closure.
  Painter.prototype.state = function () {
    return { running: this.rafId !== 0, frames: this.frames, activity: this.activity, target: this.target,
      light: this.light(), canMove: this.canMove(), size: [this.cv.width, this.cv.height], shown: this.cv.style.opacity !== '0' };
  };

  var api = {
    PRESETS: PRESETS, PRESET_KEYS: PRESET_KEYS, STYLES: STYLES, PALETTES: PALETTES, LIMITS: LIMITS, DEFAULT_LOOK: DEFAULT_LOOK,
    IDLE_MS: IDLE_MS,
    hexRGB: hexRGB, soften: soften, normalizeLook: normalizeLook, feelOf: feelOf, matchPreset: matchPreset,
    wordI: wordI, wordS: wordS, noise: noise, fbm: fbm, sample: sample, sampleStill: sampleStill,
    deviceIsLimited: deviceIsLimited, canvasSize: canvasSize, daysClip: daysClip,
    create: function (host, opts) { return new Painter(host, opts); }
  };
  if (typeof window !== 'undefined') {
    window.Chronicle = window.Chronicle || {};
    window.Chronicle.calendarEraBlend = api;
  }
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})();
