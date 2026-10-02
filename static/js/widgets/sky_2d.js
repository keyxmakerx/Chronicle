/*
 * sky_2d.js — the basic sky (window.Sky2D): the design contract's own 2D
 * fallback painter, ported closely. It draws whenever the painted sky
 * (sky_gl.js) can't: the palette's gradient and glow, stars, the sun, MOONR
 * moon sprites turned toward the sun and eclipse-shadowed, a land
 * silhouette, a cloud wash, a still of rain/snow, and fog, plus soft
 * overlays for the day's sky events (blood wash, conjunction thread,
 * harvest glow; see drawEventFX). The painted sky has the full versions.
 *
 * LAND has no terrain data to draw from, so it's generated from a stable
 * per-calendar seed, so different calendars don't look identical.
 */
(function () {
  'use strict';

  var TAU = Math.PI * 2, W = window.SkyWorld, LOOKS = window.SkyLooks, clamp = W.clamp, smooth01 = W.smooth01;

  function h(i, k) { var x = Math.sin(i * 12.9898 + k * 78.233) * 43758.5453; return x - Math.floor(x); }
  function css(c, a) { return LOOKS.srgb(c, a == null ? 1 : a); }

  // ── Land: four parallax ridge layers, generated once per (size, seed) and
  // cached as a canvas, same caching shape as the contract's own land(). ──
  var LANDC = [];
  function ridgeAt(seed, layer, x) {
    var freq = .55 + layer * .35, i = Math.floor(x * freq), f = x * freq - i;
    var a = h(i, seed + layer * 7.1), b = h(i + 1, seed + layer * 7.1);
    var u = f * f * (3 - 2 * f);
    return a + (b - a) * u;
  }
  function land(Wpx, Hpx, dpr, P, seed) {
    var key = [Wpx.toFixed(1), Hpx.toFixed(1), dpr, seed, P.h0, P.h1, P.h2, P.h3, P.rim].map(function (v) { return Array.isArray(v) ? v.map(function (x) { return x.toFixed(3); }).join(',') : v; }).join('|');
    for (var i = 0; i < LANDC.length; i++) if (LANDC[i].key === key) return LANDC[i].cv;
    var cv = document.createElement('canvas'); cv.width = Math.max(1, Math.round(Wpx * dpr)); cv.height = Math.max(1, Math.round(Hpx * dpr));
    var ctx = cv.getContext('2d'), cols = [P.h0, P.h1, P.h2, P.h3];
    ctx.scale(cv.width / Wpx, cv.height / Hpx);
    var N = Math.max(24, Math.round(Wpx / 6));
    for (var k = 0; k < 4; k++) {
      var base = .82 + k * .045, amp = .09 - k * .012;
      ctx.beginPath(); ctx.moveTo(0, Hpx);
      for (var j = 0; j < N; j++) { var x = (j / (N - 1)) * Wpx, yy = (base - amp * ridgeAt(seed, k, j / N)) * Hpx; ctx.lineTo(x, yy); }
      ctx.lineTo(Wpx, Hpx); ctx.closePath();
      var g = ctx.createLinearGradient(0, Hpx * .6, 0, Hpx);
      g.addColorStop(0, css(cols[k])); g.addColorStop(1, css(cols[k].map(function (v) { return v * .8; })));
      ctx.fillStyle = g; ctx.fill();
      ctx.strokeStyle = css(P.rim || cols[k], .32 * (1 - k * .2)); ctx.lineWidth = 1; ctx.beginPath();
      for (j = 0; j < N; j++) { var x2 = (j / (N - 1)) * Wpx, y2 = (base - amp * ridgeAt(seed, k, j / N)) * Hpx + .5; if (j) ctx.lineTo(x2, y2); else ctx.moveTo(x2, y2); }
      ctx.stroke();
    }
    LANDC.unshift({ key: key, cv: cv }); if (LANDC.length > 8) LANDC.pop();
    return cv;
  }

  // drawEventFX: the day's sky events, drawn after the moons and before the
  // land (so a blood wash sits behind the hills, matching the mockup's own
  // layering of its drawn layer). st.overlay is SkyEvents.overlaysFor()'s
  // result, attached by the state builder.
  function drawEventFX(ctx, st) {
    var ov = st.overlay; if (!ov) return;
    var through = clamp((1 - st.wx.cloud * .85) * (1 - st.wx.fog * .6), .12, 1);
    // Conjunction: a soft thread of light rim-to-rim between the grouped moons.
    (ov.conjGroups || []).forEach(function (ids) {
      var ms = ids.map(function (id) { return st.moons.filter(function (M) { return M.mo.id === id; })[0]; }).filter(function (m) { return m && m.up; });
      ms.sort(function (a, b) { return a.x - b.x; });
      for (var i = 0; i + 1 < ms.length; i++) {
        var a = ms[i], b = ms[i + 1], dx = b.x - a.x, dy = b.y - a.y, d = Math.hypot(dx, dy);
        if (d <= a.r + b.r + 2) continue;
        var ux = dx / d, uy = dy / d, x0 = a.x + ux * a.r, y0 = a.y + uy * a.r, x1 = b.x - ux * b.r, y1 = b.y - uy * b.r;
        var g = ctx.createLinearGradient(x0, y0, x1, y1);
        g.addColorStop(0, css(a.mo.color ? W.PAL.hexLin(a.mo.color) : [1, 1, 1], .85 * through));
        g.addColorStop(1, css(b.mo.color ? W.PAL.hexLin(b.mo.color) : [1, 1, 1], .85 * through));
        ctx.save(); ctx.lineCap = 'round'; ctx.strokeStyle = g; ctx.lineWidth = 1.4; ctx.beginPath(); ctx.moveTo(x0, y0); ctx.lineTo(x1, y1); ctx.stroke(); ctx.restore();
      }
    });
    // Harvest: a warm halo behind the moon, strongest low over the hills.
    Object.keys(ov.harvest || {}).forEach(function (id) {
      var m = st.moons.filter(function (M) { return M.mo.id === Number(id) || M.mo.id === id; })[0];
      if (!m || !m.up) return;
      var low = smooth01(24 * W.D2R, 2 * W.D2R, m.m.alt), rg = ctx.createRadialGradient(m.x, m.y, 0, m.x, m.y, m.r * 3.2);
      rg.addColorStop(0, css([1, .74, .38], .35 * (.4 + .6 * low) * through)); rg.addColorStop(1, css([1, .74, .38], 0));
      ctx.fillStyle = rg; ctx.fillRect(m.x - m.r * 3.2, m.y - m.r * 3.2, m.r * 6.4, m.r * 6.4);
    });
    // Blood: a slow-pulsing red wash across the sky, keyed off the darkest
    // (most total) blood moon currently up — the disc itself already turns
    // to congealed blood via MOONR's own `blood` shading; this adds the
    // "the sky bleeds with it" atmosphere at a fraction of the contract's
    // full rivulet simulation.
    var bloodIDs = Object.keys(ov.blood || {});
    if (bloodIDs.length) {
      var pulse = .5 + .5 * Math.sin(st.t * .6);
      var g2 = ctx.createRadialGradient(st.W / 2, st.L.hor, 0, st.W / 2, st.L.hor, st.H * 1.1);
      g2.addColorStop(0, css([.55, .05, .06], (.16 + .05 * pulse) * through)); g2.addColorStop(1, css([.55, .05, .06], 0));
      ctx.fillStyle = g2; ctx.fillRect(0, 0, st.W, st.H);
    }
  }

  var X = {};
  // draw(ctx, st, dpr, oy): st is the state builder's per-frame object (see
  // sky_pane.js's buildState) — the same shape the contract's Surface.state
  // produces; it reads only what a 2D picture can show. oy (CSS px,
  // default 0) lowers the whole picture, so a folding pane draws its sky
  // whole into the strip that shows rather than cropping a bigger one.
  X.draw = function (ctx, st, dpr, oy) {
    var P = st.P, L = st.L, Wpx = st.W, H = st.H, wx = st.wx, t = st.t;
    ctx.save();
    ctx.setTransform(dpr, 0, 0, dpr, 0, (oy || 0) * dpr);
    ctx.clearRect(0, 0, Wpx, H);
    var g = ctx.createLinearGradient(0, L.hor, 0, L.top);
    g.addColorStop(0, css(P.hor)); g.addColorStop(.45, css(P.mid)); g.addColorStop(1, css(P.zen));
    ctx.fillStyle = g; ctx.fillRect(0, 0, Wpx, H);
    var glowK = .55 * (1 - wx.cloud * .7);
    if (glowK > .02) {
      var gy = Math.max(st.sp.y, L.hor - H * .1), gr = ctx.createRadialGradient(st.sp.x, gy, 0, st.sp.x, gy, H * 1.1);
      gr.addColorStop(0, css(P.glow, .7 * glowK)); gr.addColorStop(1, css(P.glow, 0)); ctx.fillStyle = gr; ctx.fillRect(0, 0, Wpx, H);
    }
    var sv = smooth01(-3, -13, st.altEff) * (1 - smooth01(.3, .85, wx.cloud)) * (1 - wx.fog);
    if (sv > .02) {
      var turn = W.mod(st.sidereal + (st.cam || 0), 360), n = Math.round(Wpx * H / 700);
      for (var i = 0; i < n; i++) {
        var lon = h(i, 1) * 360, v = h(i, 2), x = W.mod((lon - turn) / 270 + .5, 360 / 270) * Wpx, y = L.hor - v * (L.hor - L.top), m = Math.pow(h(i, 3), 5);
        if (x > Wpx) continue;
        ctx.fillStyle = 'rgba(236,240,255,' + (sv * (.25 + .75 * m)).toFixed(3) + ')'; ctx.fillRect(x, y, .6 + m * 1.2, .6 + m * 1.2);
      }
    }
    if (st.alt > -1 * W.D2R) { var sr = st.R * .95, sk = (1 - wx.cloud * .82); ctx.fillStyle = 'rgba(255,246,222,' + sk.toFixed(3) + ')'; ctx.beginPath(); ctx.arc(st.sp.x, st.sp.y, sr, 0, TAU); ctx.fill(); }
    var through = clamp((1 - wx.cloud * .9) * (1 - wx.fog * .75), .04, 1);
    st.moons.forEach(function (M) {
      if (!M.up) return;
      // M.sr, when set, is the moon's resting radius: a folding pane scales
      // that sprite rather than shading a new one at every size it passes.
      var sz = Math.max(8, Math.ceil((M.sr || M.r) * 2 * dpr / 8) * 8), spr = window.MOONR.sprite(M.spec, M.m.p, sz, { blood: M.blood });
      ctx.save(); ctx.globalAlpha = through; ctx.translate(M.x, M.y); ctx.rotate(M.rot); ctx.drawImage(spr, -M.r, -M.r, M.r * 2, M.r * 2);
      if (M.shadow) { ctx.rotate(-M.rot); ctx.beginPath(); ctx.arc(0, 0, M.r, 0, TAU); ctx.clip(); ctx.fillStyle = 'rgba(40,12,8,' + (.85 * M.shadow[3]).toFixed(3) + ')'; ctx.beginPath(); ctx.arc(M.shadow[0], M.shadow[1], M.shadow[2], 0, TAU); ctx.fill(); }
      ctx.restore();
    });
    drawEventFX(ctx, st);
    if (wx.cloud > .15) {
      var cc = P.clit || [.8, .8, .82];
      for (var c = 0; c < 9; c++) { var cx = Wpx * h(c, 7), cy = H * (.12 + .45 * h(c, 8)), rx = Wpx * (.12 + .12 * h(c, 9)); var cg = ctx.createRadialGradient(cx, cy, 0, cx, cy, rx); cg.addColorStop(0, css(cc, .55 * wx.cloud)); cg.addColorStop(1, css(cc, 0)); ctx.fillStyle = cg; ctx.fillRect(cx - rx, cy - rx, rx * 2, rx * 2); }
    }
    ctx.drawImage(land(Wpx, H, dpr, P, st.landSeed || 1), 0, 0, Wpx, H);
    if (wx.rain > .05) { ctx.strokeStyle = 'rgba(210,222,240,' + (.25 * Math.min(1, wx.rain)).toFixed(3) + ')'; ctx.lineWidth = .8; ctx.beginPath(); for (i = 0; i < Wpx * H / 900 * Math.min(1.6, wx.rain); i++) { var rx0 = h(i, 11) * Wpx, ry0 = h(i, 12) * H; ctx.moveTo(rx0, ry0); ctx.lineTo(rx0 - 2, ry0 + 9); } ctx.stroke(); }
    if (wx.snow > .05) { ctx.fillStyle = 'rgba(244,247,255,' + (.7 * Math.min(1, wx.snow)).toFixed(3) + ')'; for (i = 0; i < Wpx * H / 700 * Math.min(1.6, wx.snow); i++) { ctx.beginPath(); ctx.arc(h(i, 13) * Wpx, h(i, 14) * H, .6 + h(i, 15) * 1.4, 0, TAU); ctx.fill(); } }
    if (wx.fog > .05) { ctx.fillStyle = css(P.fog, .5 * wx.fog); ctx.fillRect(0, 0, Wpx, H); }
    ctx.restore();
  };
  window.Sky2D = X;
})();
