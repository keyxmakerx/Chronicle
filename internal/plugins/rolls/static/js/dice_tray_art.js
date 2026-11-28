/**
 * dice_tray_art.js: the flat, drawn art of the dice trays (Chronicle.diceTrayArt).
 *
 * Everything here is 2D canvas painting. dice_tray_stage.js lays each picture on a flat, unlit plane
 * where the real wall stands, so the 3D dice pass behind the paper tower's front and show only
 * through its window and arch. A look is Simple (the app's own tokens), Flat paper (card stock and
 * ink lines), or Detailed in Paper or in the style kit's tokens. Paper stays paper in dark mode:
 * the same stock, a little dimmer, never inverted (static/css/paper.css).
 *
 * Self-contained so the Foundry module can load it unchanged.
 */
(function () {
  'use strict';
  var W = typeof window !== 'undefined' ? window : globalThis;
  W.Chronicle = W.Chronicle || {};

function cv(w, h) { var c = document.createElement('canvas'); c.width = Math.max(2, Math.round(w)); c.height = Math.max(2, Math.round(h)); return c; }
function rnd(seed) { var a = seed >>> 0; return function () { a |= 0; a = a + 0x6D2B79F5 | 0; var t = Math.imul(a ^ a >>> 15, 1 | a); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }; }
function mix(a, b, t) {
  var x = parseInt(a.slice(1), 16), y = parseInt(b.slice(1), 16);
  function ch(sh) { return Math.round((x >> sh & 255) * (1 - t) + (y >> sh & 255) * t); }
  return '#' + ((1 << 24) + (ch(16) << 16) + (ch(8) << 8) + ch(0)).toString(16).slice(1);
}
function rgba(hex, a) { var n = parseInt(hex.slice(1), 16); return 'rgba(' + (n >> 16) + ',' + (n >> 8 & 255) + ',' + (n & 255) + ',' + a + ')'; }
function lum(hex) { var n = parseInt(hex.slice(1), 16); return ((n >> 16) * 0.3 + (n >> 8 & 255) * 0.59 + (n & 255) * 0.11) / 255; }

// Paper grain: speckle and a few long fibres, added only where something is drawn.
function grain(c, amt, fibres, seed) {
  var g = c.getContext('2d'), w = c.width, h = c.height, id = g.getImageData(0, 0, w, h), d = id.data, r = rnd(seed || 9);
  for (var i = 0; i < d.length; i += 4) { if (!d[i + 3]) continue; var n = (r() - 0.5) * amt; d[i] += n; d[i + 1] += n; d[i + 2] += n; }
  g.putImageData(id, 0, 0);
  if (!fibres) return;
  g.save(); g.globalCompositeOperation = 'source-atop'; g.lineWidth = 1;
  for (var f = 0; f < fibres * w * h / 65536; f++) {
    var x = r() * w, y = r() * h, a = r() * 6.28, l = 6 + r() * 18;
    g.strokeStyle = r() > 0.5 ? 'rgba(255,255,255,.18)' : 'rgba(0,0,0,.07)';
    g.beginPath(); g.moveTo(x, y); g.quadraticCurveTo(x + Math.cos(a + 0.6) * l / 2, y + Math.sin(a + 0.6) * l / 2, x + Math.cos(a) * l, y + Math.sin(a) * l); g.stroke();
  }
  g.restore();
}

// ---------- Looks ----------
// kind: 'simple' (a plain app panel in the page's tokens), 'flat' (flat paper: card stock with ink
// lines and no texture), or the detailed tower's finish, 'paper' or 'kit'. tok holds the page's colours
// as hex; kit is the style kit's name.
var KIT_R = { material: 0.12, sharp: 0, ornate: 0.06, flowing: 0.42, lumen: 0.14, sketchbook: 0.05 };
function lookFrom(kind, tok, kit, dark, floor) {
  var L = { kind: kind, kit: kit, dark: dark, floor: floor || 'paper', accent: tok.accent };
  if (kind === 'simple') {
    // One or two flat tones, a hairline border and a soft shadow; the kit changes only these values.
    var s0 = tok.surface, a0 = tok.alt, b0 = tok.border, t0 = tok.text;
    Object.assign(L, { card: s0, card2: a0, edge: b0, ink: mix(b0, t0, dark ? 0.25 : 0.35), inner: mix(a0, t0, dark ? 0.2 : 0.16), base: a0, light: s0,
      shadow: dark ? 'rgba(0,0,0,.45)' : 'rgba(20,24,40,.12)', burn: 'rgba(0,0,0,0)', burnHex: t0, felt: mix(a0, tok.accent, dark ? 0.22 : 0.16) });
    L.r = KIT_R[kit] == null ? 0.12 : KIT_R[kit]; L.lw = 1; L.grain = 0; L.fibres = 0; L.pattern = null; L.simple = true;
    return L;
  }
  if (kind === 'paper' || kind === 'flat') {
    // The shared paper tokens (static/css/paper.css). Paper stays paper in dark mode: the same stock in
    // low light, a little dimmer, with deeper shadows, never inverted.
    var P = { paper: '#efe6d2', cut: '#f8f2e4', under: '#e2d5b8', edge: '#c4b89a', ink: '#4a4152', burn: '#6e5028' };
    var dim = dark ? function (c) { return mix(c, '#14151b', 0.2); } : function (c) { return c; };
    Object.assign(L, { card: dim(P.paper), card2: dim(P.under), edge: dim(P.edge), ink: mix(P.ink, P.burn, 0.45), inner: dim(mix(P.under, P.burn, 0.5)), base: dim(P.paper), light: dim(P.cut),
      shadow: dark ? 'rgba(0,0,0,.7)' : 'rgba(90,62,28,.42)', burn: dark ? 'rgba(60,40,15,.38)' : 'rgba(110,80,40,.26)', burnHex: P.burn, felt: '#2d5b50' });
    L.r = 0.05; L.lw = 1.1; L.grain = 14; L.fibres = 1; L.pattern = 'stone';
    // Flat paper: the same stock and shapes with ink lines only, no grain, burn or highlights.
    if (kind === 'flat') { L.kind = 'paper'; L.plain = true; L.grain = 0; L.fibres = 0; L.pattern = 'courses'; L.lw = 1.3; }
  } else {
    var s = tok.surface, a = tok.alt, b = tok.border, t = tok.text;
    Object.assign(L, { card: mix(s, a, 0.35), card2: mix(a, t, dark ? 0.1 : 0.08), edge: mix(b, t, 0.15), ink: mix(b, t, dark ? 0.3 : 0.5), inner: mix(a, t, dark ? 0.3 : 0.32), base: a, light: mix(s, '#ffffff', dark ? 0.08 : 0.6),
      shadow: dark ? 'rgba(0,0,0,.6)' : 'rgba(20,24,40,.24)', felt: mix(a, tok.accent, dark ? 0.22 : 0.16) });
    var K = [KIT_R[kit] == null ? 0.12 : KIT_R[kit], { sharp: 2, ornate: 1.2, sketchbook: 1.6 }[kit] || 1];
    L.r = K[0]; L.lw = K[1]; L.grain = kit === 'sketchbook' ? 10 : 3; L.fibres = kit === 'sketchbook' ? 0.5 : 0;
    L.burn = dark ? 'rgba(0,0,0,.3)' : rgba(t, 0.12); L.burnHex = t; L.pattern = kit === 'ornate' ? 'stone' : kit === 'sharp' || kit === 'lumen' ? null : 'lines';
    L.sharp = kit === 'sharp'; L.ornate = kit === 'ornate'; L.flowing = kit === 'flowing'; L.lumen = kit === 'lumen'; L.sketch = kit === 'sketchbook';
  }
  return L;
}

// Rounded rectangle in canvas units; r is clamped to the shorter side.
function rr(g, x, y, w, h, r) { r = Math.max(0, Math.min(r, w / 2, h / 2)); g.moveTo(x + r, y); g.arcTo(x + w, y, x + w, y + h, r); g.arcTo(x + w, y + h, x, y + h, r); g.arcTo(x, y + h, x, y, r); g.arcTo(x, y, x + w, y, r); g.closePath(); }
// A soft shadow under whatever fill() draws next; Sharp casts a hard offset one instead.
function shadowOn(g, L, k, depth) {
  if (L.sharp) { g.shadowColor = rgba(L.edge, 0.9); g.shadowBlur = 0; g.shadowOffsetX = 0.04 * k * depth; g.shadowOffsetY = 0.04 * k * depth; return; }
  g.shadowColor = L.shadow; g.shadowBlur = 0.12 * k * depth; g.shadowOffsetX = 0.015 * k * depth; g.shadowOffsetY = 0.04 * k * depth;
}
function shadowOff(g) { g.shadowColor = 'transparent'; g.shadowBlur = 0; g.shadowOffsetX = 0; g.shadowOffsetY = 0; }
// The outline a look draws: ink for paper, the border token for a kit, doubled by hand for Sketchbook
// and given a glow for Lumen.
function ink(g, L, k, w) {
  g.lineWidth = (w || L.lw) * Math.max(1, k / 60); g.strokeStyle = L.ink;
  if (L.lumen) { g.save(); g.shadowColor = L.accent; g.shadowBlur = 0.12 * k; g.strokeStyle = rgba(L.accent, 0.8); g.stroke(); g.restore(); return; }
  g.stroke();
  if (L.sketch) { g.save(); g.translate(0.012 * k, -0.01 * k); g.globalAlpha = 0.55; g.stroke(); g.restore(); }
}
function hatch(g, L, k, x, y, w, h) {
  g.save(); g.clip(); g.strokeStyle = rgba(L.ink, 0.28); g.lineWidth = Math.max(1, k / 70);
  for (var i = -h; i < w; i += 0.09 * k) { g.beginPath(); g.moveTo(x + i, y + h); g.lineTo(x + i + h, y); g.stroke(); }
  g.restore();
}

// ---------- The paper tower ----------
// d: the tower's layout (tray2.js towerLayout). k: canvas pixels per world unit. Returns each picture
// with the world rectangle it covers, so tray2.js can lay it exactly over the physics.
// The front is a facade of layered card, wider than the tower's walls behind it: a back sheet, the face
// with its printed stonework, pilasters, a cornice and merlons, a framed window and a voussoir arch.
// Every layer casts a soft shadow on the one behind and darkens a little at its cut edges.
var CAP = 0.32, MER = 0.72, OVER = 0.42;
function towerShape(d) {
  var ww = d.IW - 0.3, wy0 = 2.15, wy1 = d.H - 0.75;
  return { ww: ww, wy0: wy0, wy1: wy1, ax: d.AW, ay: d.AH - 0.55, ary: 0.55 };
}
function burnEdge(g, L, k, path, wgt) {
  g.save(); g.beginPath(); path(); g.clip('evenodd');
  g.filter = 'blur(' + Math.max(1, Math.round(0.05 * k)) + 'px)';
  g.lineWidth = (wgt || 0.11) * k; g.strokeStyle = L.burn; g.beginPath(); path(); g.stroke();
  g.restore();
}
function towerArt(L, d, k) {
  var tr = d.tray, W = d.IW + d.t, FX0 = d.Tx - W - OVER, FX1 = d.Tx + W + OVER, CX0 = FX0 - 0.12, CX1 = FX1 + 0.12;
  var YT = d.H + CAP, YTOP = YT + MER, S = towerShape(d), PW = 0.36, HB = d.H - 0.12;
  function X(x) { return (x - CX0) * k; } function Y(y) { return (YTOP - y) * k; }
  function windowPath(g, grow) {
    var e = grow || 0, l = X(d.Tx - S.ww - e), r = X(d.Tx + S.ww + e), b = Y(S.wy0 - e), rad = (S.ww + e) * k, top = Y(S.wy1 - S.ww);
    g.moveTo(l, b); g.lineTo(l, top); g.arc(X(d.Tx), top, rad, Math.PI, 0); g.lineTo(r, b); g.closePath();
  }
  function archPath(g, grow) {
    var e = grow || 0, l = X(d.Tx - S.ax - e), r = X(d.Tx + S.ax + e), top = Y(S.ay);
    g.moveTo(l, Y(-0.01)); g.lineTo(l, top); g.ellipse(X(d.Tx), top, (S.ax + e) * k, (S.ary + e) * k, 0, Math.PI, 0); g.lineTo(r, Y(-0.01)); g.closePath();
  }
  function rect(g, x0, y0, x1, y1, r) { rr(g, X(x0), Y(y1), (x1 - x0) * k, (y1 - y0) * k, r == null ? L.r * k : r); }
  // One layer of card: shadow on what is behind, the fill, burnt cut edges, the outline.
  function layer(g, path, fill, depth, opt) {
    opt = opt || {};
    g.save(); shadowOn(g, L, k, L.plain ? depth * 0.5 : depth); g.beginPath(); path(); g.fillStyle = fill; g.fill('evenodd'); g.restore();
    if (opt.pattern) opt.pattern();
    if ((L.kind === 'paper' && !L.plain) || L.sketch) burnEdge(g, L, k, path, opt.burn);
    if (L.sketch && opt.hatch) { g.beginPath(); path(); hatch(g, L, k, 0, 0, g.canvas.width, g.canvas.height); }
    g.beginPath(); path(); ink(g, L, k, opt.lw);
    if (!L.sharp && !L.lumen && !L.plain) { g.save(); g.beginPath(); path(); g.clip('evenodd'); g.translate(0.012 * k, 0.016 * k); g.beginPath(); path(); g.lineWidth = Math.max(1, k / 70); g.strokeStyle = rgba(L.light, 0.75); g.stroke(); g.restore(); }
  }
  // A dashed score line where the card is folded.
  function score(g, x0, y0, x1, y1) { g.save(); g.setLineDash([0.07 * k, 0.05 * k]); g.lineWidth = Math.max(1, k / 90); g.strokeStyle = rgba(L.ink, 0.5); g.beginPath(); g.moveTo(X(x0), Y(y0)); g.lineTo(X(x1), Y(y1)); g.stroke(); g.restore(); }
  var out = {}, c = cv((CX1 - CX0) * k, YTOP * k), g = c.getContext('2d'), R = L.r * k * 1.6;
  var holes = function (e) { windowPath(g, e); archPath(g, e); };

  if (L.simple) {
    // Simple: one panel with the window and arch cut out, a quiet band round each, hairline borders.
    var sr = Math.max(0.03, L.r) * k, hl = Math.max(1, k / 80);
    g.save(); shadowOn(g, L, k, 0.5); g.beginPath(); rect(g, FX0, 0, FX1, YT, sr); holes(0); g.fillStyle = L.card; g.fill('evenodd'); g.restore();
    [windowPath, archPath].forEach(function (p) { g.beginPath(); p(g, 0.16); p(g, 0); g.fillStyle = L.card2; g.fill('evenodd'); g.beginPath(); p(g, 0.16); g.lineWidth = hl; g.strokeStyle = L.edge; g.stroke(); });
    g.beginPath(); rect(g, FX0, 0, FX1, YT, sr); holes(0); g.lineWidth = hl; g.strokeStyle = L.edge; g.stroke();
  } else {
  // 1. The back sheet: the facade's full shape, darker card.
  layer(g, function () { rect(g, FX0, 0, FX1, HB, R); holes(0.03); }, mix(L.card2, L.burnHex, 0.22), 1.4);
  // 2. The face, inset on it, printed with fine stonework.
  var fx0 = FX0 + 0.1, fx1 = FX1 - 0.1;
  var facePath = function () { rect(g, fx0, 0, fx1, HB - 0.08, R * 0.7); holes(0.08); };
  var faceFill = g.createLinearGradient(0, Y(HB), 0, Y(0)); faceFill.addColorStop(0, L.light); faceFill.addColorStop(0.35, L.card); faceFill.addColorStop(1, mix(L.card, L.card2, 0.35));
  layer(g, facePath, faceFill, 0.7, { burn: 0.16, hatch: false, pattern: function () {
    if (!L.pattern) return;
    g.save(); g.beginPath(); facePath(); g.clip('evenodd');
    var rn = rnd(21), ch = 0.3, row = 0;
    for (var y = 0; y < HB; y += ch, row++) {
      var off = row % 2 ? 0.31 : 0, bw = 0.62;
      for (var x = fx0 - off; x < fx1; x += bw) {
        var t = rn();
        if (L.pattern === 'stone') { g.fillStyle = t > 0.5 ? rgba(L.light, 0.22 * (t - 0.5)) : rgba(L.burnHex, 0.12 * (0.5 - t)); g.fillRect(X(x), Y(y + ch), bw * k, ch * k); }
        g.strokeStyle = rgba(L.ink, L.pattern === 'lines' ? 0.1 : 0.2); g.lineWidth = Math.max(0.8, k / 110);
        g.beginPath(); g.moveTo(X(x), Y(y)); g.lineTo(X(x), Y(y + ch)); g.stroke();
        if (L.pattern === 'stone' && t > 0.7) { g.beginPath(); g.moveTo(X(x + bw * 0.2), Y(y + ch * 0.35)); g.lineTo(X(x + bw * 0.55), Y(y + ch * 0.3)); g.strokeStyle = rgba(L.ink, 0.1); g.stroke(); }
      }
      g.strokeStyle = rgba(L.ink, L.pattern === 'lines' ? 0.12 : 0.24); g.beginPath(); g.moveTo(X(fx0), Y(y)); g.lineTo(X(fx1), Y(y)); g.stroke();
    }
    g.restore();
  } });
  // 3. Pilasters at each side, with a fold down their middle, a plinth and a capital.
  [[FX0, FX0 + PW], [FX1 - PW, FX1]].forEach(function (p) {
    var pg = g.createLinearGradient(X(p[0]), 0, X(p[1]), 0); pg.addColorStop(0, L.light); pg.addColorStop(0.5, L.card); pg.addColorStop(0.5, mix(L.card, L.card2, 0.6)); pg.addColorStop(1, L.card2);
    layer(g, function () { rect(g, p[0], 0, p[1], HB - 0.02, L.r * k * 0.5); }, pg, 1.3, { hatch: true });
    score(g, (p[0] + p[1]) / 2, 0.5, (p[0] + p[1]) / 2, HB - 0.45);
    layer(g, function () { rect(g, p[0] - 0.05, 0, p[1] + 0.05, 0.42, L.r * k * 0.4); }, L.card2, 0.8);
    layer(g, function () { rect(g, p[0] - 0.05, HB - 0.36, p[1] + 0.05, HB - 0.1, L.r * k * 0.4); }, L.card2, 0.8);
  });
  // 4. The window: an outer frame with small cut-outs, an inner frame, a sill on corbels and a hood.
  var wf = function (e0, e1) { return function () { windowPath(g, e0); windowPath(g, e1); }; };
  layer(g, wf(0.3, 0.02), mix(L.card2, L.burnHex, 0.08), 1.4);
  var ac = 0.17, n = 9;
  for (var i = 0; i < n; i++) {
    var a = Math.PI + (i + 0.5) / n * Math.PI, cx = X(d.Tx) + Math.cos(a) * (S.ww + ac) * k, cy = Y(S.wy1 - S.ww) + Math.sin(a) * (S.ww + ac) * k;
    g.beginPath(); if (i % 2) g.arc(cx, cy, 0.05 * k, 0, 6.3); else { g.moveTo(cx, cy - 0.085 * k); g.lineTo(cx + 0.06 * k, cy); g.lineTo(cx, cy + 0.085 * k); g.lineTo(cx - 0.06 * k, cy); g.closePath(); }
    g.fillStyle = L.ornate ? L.accent : mix(L.inner, '#000000', 0.25); g.fill();
  }
  [S.wy0 + 0.5, S.wy0 + 1.6].forEach(function (y) { [-1, 1].forEach(function (s) {
    var cx = X(d.Tx + s * (S.ww + ac)), cy = Y(y); g.beginPath(); g.arc(cx, cy, 0.05 * k, 0, 6.3); g.fillStyle = L.ornate ? L.accent : mix(L.inner, '#000000', 0.25); g.fill();
  }); });
  layer(g, wf(0.12, 0), L.light, 0.8, { lw: 1 });
  // The hood over the window's arch, ending in two small drops.
  var hood = function () { var r0 = (S.ww + 0.3) * k, r1 = (S.ww + 0.44) * k, cx = X(d.Tx), cy = Y(S.wy1 - S.ww);
    g.moveTo(cx - r1, cy + 0.12 * k); g.arc(cx, cy, r1, Math.PI, 0); g.lineTo(cx + r1, cy + 0.12 * k); g.lineTo(cx + r0, cy + 0.12 * k); g.arc(cx, cy, r0, 0, Math.PI, true); g.closePath(); };
  if (!L.plain) layer(g, hood, L.card, 1);
  // The window's keystone, two pieces.
  var wk = function (y, w0, w1, h) { return function () { var x = X(d.Tx), t = Y(y); g.moveTo(x - w0 * k, t); g.lineTo(x + w0 * k, t); g.lineTo(x + w1 * k, t + h * k); g.lineTo(x - w1 * k, t + h * k); g.closePath(); }; };
  layer(g, wk(S.wy1 + 0.5, 0.17, 0.11, 0.48), L.card2, 1.1);
  if (!L.plain) layer(g, wk(S.wy1 + 0.44, 0.1, 0.07, 0.34), L.ornate ? L.accent : L.light, 0.6, { lw: 0.8 });
  // Sill and corbels.
  if (!L.plain) [-1, 1].forEach(function (s) { layer(g, function () { var x = d.Tx + s * (S.ww - 0.05); rect(g, x - 0.12, S.wy0 - 0.5, x + 0.12, S.wy0 - 0.18, L.r * k * 0.3); }, L.card2, 0.8); });
  layer(g, function () { rect(g, d.Tx - S.ww - 0.45, S.wy0 - 0.2, d.Tx + S.ww + 0.45, S.wy0, L.r * k * 0.3); }, L.card, 1.2);
  // Brads holding the frame's layers.
  if (!L.sharp && !L.flowing && !L.plain) [-1, 1].forEach(function (s) {
    var bx = X(d.Tx + s * (S.ww + 0.16)), by = Y(S.wy0 + 0.14), br = 0.05 * k;
    g.beginPath(); g.arc(bx, by, br, 0, 6.3); g.fillStyle = L.ornate ? L.accent : mix(L.edge, L.ink, 0.45); g.fill();
    g.beginPath(); g.arc(bx - br * 0.3, by - br * 0.3, br * 0.35, 0, 6.3); g.fillStyle = rgba(L.light, 0.85); g.fill();
  });
  // 5. The arch: jambs, impost blocks, a ring of voussoirs and a layered keystone.
  [-1, 1].forEach(function (s) {
    var x = d.Tx + s * S.ax;
    layer(g, function () { var a0 = s < 0 ? x - 0.24 : x, a1 = s < 0 ? x : x + 0.24; rect(g, a0, 0, a1, S.ay, 0); }, L.card2, 0.6);
    layer(g, function () { var a0 = s < 0 ? x - 0.34 : x - 0.04, a1 = s < 0 ? x + 0.04 : x + 0.34; rect(g, a0, S.ay - 0.02, a1, S.ay + 0.14, L.r * k * 0.3); }, L.card, 0.8);
  });
  var VN = 11, vin = 0.02, vout = 0.36, ecx = X(d.Tx), ecy = Y(S.ay + 0.14);
  for (var v = 0; v < VN; v++) {
    if (v === (VN - 1) / 2) continue;
    (function (v) {
      var a0 = Math.PI + v / VN * Math.PI + 0.006, a1 = Math.PI + (v + 1) / VN * Math.PI - 0.006;
      var vp = function () {
        g.moveTo(ecx + Math.cos(a0) * (S.ax + vin) * k, ecy + Math.sin(a0) * (S.ary + vin) * k);
        g.lineTo(ecx + Math.cos(a0) * (S.ax + vout) * k, ecy + Math.sin(a0) * (S.ary + vout) * k);
        g.lineTo(ecx + Math.cos(a1) * (S.ax + vout) * k, ecy + Math.sin(a1) * (S.ary + vout) * k);
        g.lineTo(ecx + Math.cos(a1) * (S.ax + vin) * k, ecy + Math.sin(a1) * (S.ary + vin) * k); g.closePath();
      };
      layer(g, vp, v % 2 ? L.card : mix(L.card, L.card2, 0.6), 0.7, { lw: 0.9, burn: 0.07, hatch: v % 2 === 0 });
    })(v);
  }
  var kt = Y(S.ay + 0.14 + S.ary + vout + 0.1);
  layer(g, function () { g.moveTo(ecx - 0.2 * k, kt); g.lineTo(ecx + 0.2 * k, kt); g.lineTo(ecx + 0.12 * k, ecy - S.ary * k + 0.02 * k); g.lineTo(ecx - 0.12 * k, ecy - S.ary * k + 0.02 * k); g.closePath(); }, L.card2, 1.2);
  layer(g, function () { g.moveTo(ecx - 0.12 * k, kt + 0.07 * k); g.lineTo(ecx + 0.12 * k, kt + 0.07 * k); g.lineTo(ecx + 0.08 * k, kt + 0.36 * k); g.lineTo(ecx - 0.08 * k, kt + 0.36 * k); g.closePath(); }, L.ornate ? L.accent : L.light, 0.6, { lw: 0.8 });
  // 6. The cornice: a band over the top with a row of dentils under it, folded back along its top.
  layer(g, function () { rect(g, CX0 + 0.02, HB - 0.06, CX1 - 0.02, HB + 0.12, L.r * k * 0.3); }, L.card2, 1);
  for (var dx = FX0 + 0.04; dx < FX1 - 0.1; dx += 0.22) { g.fillStyle = mix(L.inner, '#000000', 0.15); g.fillRect(X(dx), Y(HB + 0.07), 0.1 * k, 0.09 * k); }
  layer(g, function () { rect(g, CX0, HB + 0.1, CX1, YT, L.r * k * 0.4); }, L.card, 1.4);
  score(g, CX0 + 0.06, YT - 0.09, CX1 - 0.06, YT - 0.09);
  // 7. Merlons along the top, each a tab of card with an arrow slit, and a glue tab between them.
  var slots = 9, sw = (CX1 - CX0) / slots;
  for (var m = 0; m < slots; m++) {
    var mx = CX0 + m * sw;
    if (m % 2 === 0) {
      (function (mx) {
        layer(g, function () { rect(g, mx + 0.02, YT - 0.02, mx + sw - 0.02, YTOP - 0.02, L.r * k * 0.3); }, L.card, 1.2);
        g.fillStyle = mix(L.inner, '#000000', 0.3); g.fillRect(X(mx + sw / 2 - 0.025), Y(YTOP - 0.16), 0.05 * k, 0.34 * k);
      })(mx);
    } else if (!L.plain) {
      g.beginPath(); g.moveTo(X(mx + 0.06), Y(YT)); g.lineTo(X(mx + 0.12), Y(YT + 0.12)); g.lineTo(X(mx + sw - 0.12), Y(YT + 0.12)); g.lineTo(X(mx + sw - 0.06), Y(YT)); g.closePath();
      g.fillStyle = L.light; g.fill(); g.save(); g.setLineDash([0.05 * k, 0.04 * k]); g.lineWidth = Math.max(1, k / 100); g.strokeStyle = rgba(L.ink, 0.55); g.stroke(); g.restore();
    }
  }
  }
  if (L.lumen) { g.beginPath(); windowPath(g, 0.02); ink(g, L, k, 1.4); g.beginPath(); archPath(g, 0.02); ink(g, L, k, 1.4); }
  grain(c, L.grain, L.fibres, 3);
  out.front = { c: c, x0: CX0, x1: CX1, y0: 0, y1: YTOP };

  // Over the dice, inside the openings only: the shadow the card's thickness throws.
  var o = cv(c.width, c.height), og = o.getContext('2d');
  og.save(); og.beginPath(); windowPath(og, 0); og.clip();
  var gt = og.createLinearGradient(0, Y(S.wy1), 0, Y(S.wy1 - 0.5)); gt.addColorStop(0, L.shadow); gt.addColorStop(1, 'rgba(0,0,0,0)');
  og.fillStyle = gt; og.fillRect(0, Y(S.wy1 + 0.1), c.width, 0.6 * k);
  var gl = og.createLinearGradient(X(d.Tx - S.ww), 0, X(d.Tx - S.ww + 0.3), 0); gl.addColorStop(0, L.shadow); gl.addColorStop(1, 'rgba(0,0,0,0)');
  og.fillStyle = gl; og.fillRect(X(d.Tx - S.ww), 0, 0.3 * k, c.height);
  og.restore();
  og.save(); og.beginPath(); archPath(og, 0); og.clip();
  var ga = og.createLinearGradient(0, Y(S.ay + S.ary), 0, Y(S.ay - 0.3)); ga.addColorStop(0, L.shadow); ga.addColorStop(1, 'rgba(0,0,0,0)');
  og.fillStyle = ga; og.fillRect(0, Y(S.ay + S.ary), c.width, 0.9 * k); og.restore();
  out.over = { c: o, x0: CX0, x1: CX1, y0: 0, y1: YTOP };

  // Inside: the back wall seen through the window, darker towards the top where the card closes over it.
  var ic = cv(d.IW * 2 * k, d.H * k), ig = ic.getContext('2d');
  var gi = ig.createLinearGradient(0, 0, 0, ic.height); gi.addColorStop(0, mix(L.inner, '#000000', 0.35)); gi.addColorStop(0.5, L.inner); gi.addColorStop(1, mix(L.inner, '#000000', 0.2));
  ig.fillStyle = L.simple ? L.inner : gi; ig.fillRect(0, 0, ic.width, ic.height);
  var gs = ig.createLinearGradient(0, 0, ic.width, 0); gs.addColorStop(0, 'rgba(0,0,0,.25)'); gs.addColorStop(0.2, 'rgba(0,0,0,0)'); gs.addColorStop(0.8, 'rgba(0,0,0,0)'); gs.addColorStop(1, 'rgba(0,0,0,.25)');
  if (!L.simple) { ig.fillStyle = gs; ig.fillRect(0, 0, ic.width, ic.height); }
  if (L.pattern) {
    ig.strokeStyle = 'rgba(0,0,0,.14)'; ig.lineWidth = Math.max(1, k / 90);
    for (var iy = 0, row2 = 0; iy < ic.height; iy += 0.3 * k, row2++) {
      ig.beginPath(); ig.moveTo(0, iy); ig.lineTo(ic.width, iy); ig.stroke();
      for (var ix = row2 % 2 ? -0.31 * k : 0; ix < ic.width; ix += 0.62 * k) { ig.beginPath(); ig.moveTo(ix, iy); ig.lineTo(ix, iy + 0.3 * k); ig.stroke(); }
    }
  }
  grain(ic, L.grain, L.fibres, 5);
  out.inner = { c: ic, x0: d.Tx - d.IW, x1: d.Tx + d.IW, y0: 0, y1: d.H };

  // The top: the card's edge round a dark opening.
  var tz0 = d.Tz - d.ID - d.t, tz1 = d.front + d.t, tc = cv((CX1 - CX0) * k, (tz1 - tz0) * k), tg = tc.getContext('2d');
  tg.beginPath(); rr(tg, 0, 0, tc.width, tc.height, L.r * k); tg.fillStyle = L.card2; tg.fill();
  tg.fillStyle = rgba(L.light, 0.5); tg.fillRect(0, tc.height - Math.max(1, k / 40), tc.width, Math.max(1, k / 40));
  var hx0 = (d.Tx - d.IW - CX0) * k, hz0 = d.t * k, gh = tg.createLinearGradient(0, hz0, 0, hz0 + d.ID * 2 * k);
  gh.addColorStop(0, mix(L.inner, '#000000', 0.5)); gh.addColorStop(1, mix(L.inner, '#000000', 0.2));
  tg.fillStyle = L.simple ? mix(L.inner, '#000000', 0.15) : gh; tg.fillRect(hx0, hz0, d.IW * 2 * k, d.ID * 2 * k);
  tg.beginPath(); tg.rect(hx0, hz0, d.IW * 2 * k, d.ID * 2 * k); ink(tg, L, k, 1);
  tg.beginPath(); rr(tg, 0.5, 0.5, tc.width - 1, tc.height - 1, L.r * k); ink(tg, L, k, 1);
  out.top = { c: tc, x0: CX0, x1: CX1, z0: tz0, z1: tz1, y: YT };

  // The tray's floor, with the tower's shadow and the rim's inner shadow drawn on it.
  var fx0 = tr.x0 - 0.24, fx1 = tr.x1 + 0.24, fz0 = tz0 - 0.1, fz1 = tr.z1 + 0.24;
  function FX(x) { return (x - fx0) * k; } function FZ(z) { return (z - fz0) * k; }
  var fc = cv((fx1 - fx0) * k, (fz1 - fz0) * k), fg = fc.getContext('2d');
  floorFill(fg, L, fc.width, fc.height);
  // Inside the tower's footprint: shade, seen through the arch.
  fg.fillStyle = mix(L.inner, '#000000', 0.45); fg.fillRect(FX(d.Tx - W), 0, W * 2 * k, FZ(tr.z0));
  fg.save(); fg.filter = 'blur(' + Math.round(0.16 * k) + 'px)';
  fg.fillStyle = L.shadow; fg.fillRect(FX(d.Tx + W - 0.1), FZ(tr.z0 - 0.1), 1.3 * k, 0.75 * k);
  fg.fillRect(FX(tr.x0), FZ(tr.z0), (tr.x1 - tr.x0) * k, 0.22 * k);
  fg.fillRect(FX(tr.x0), FZ(tr.z0), 0.22 * k, (tr.z1 - tr.z0) * k);
  fg.restore();
  // Glue tabs where the facade's pilasters are fixed down to the tray (the detailed tower only).
  if (!L.plain && !L.simple) [[FX0, FX0 + PW], [FX1 - PW, FX1]].forEach(function (p) {
    var x0 = FX(p[0] - 0.04), x1 = FX(p[1] + 0.04), z0 = FZ(tr.z0), z1 = FZ(tr.z0 + 0.3);
    fg.beginPath(); fg.moveTo(x0, z0); fg.lineTo(x1, z0); fg.lineTo(x1 - 0.07 * k, z1); fg.lineTo(x0 + 0.07 * k, z1); fg.closePath();
    fg.save(); fg.shadowColor = L.shadow; fg.shadowBlur = 0.06 * k; fg.shadowOffsetY = 0.02 * k; fg.fillStyle = L.card2; fg.fill(); fg.restore();
    fg.save(); fg.setLineDash([0.05 * k, 0.04 * k]); fg.lineWidth = Math.max(1, k / 100); fg.strokeStyle = rgba(L.ink, 0.55); fg.stroke(); fg.restore();
  });
  grain(fc, L.floor === 'felt' ? 20 : L.grain, L.floor === 'felt' ? 0 : L.fibres, 7);
  out.floor = { c: fc, x0: fx0, x1: fx1, z0: fz0, z1: fz1 };

  // The rims' tops (one picture, open in the middle) and their faces.
  var rz0 = tr.z0 - 0.24, rc = cv((fx1 - fx0) * k, (fz1 - rz0) * k), rg = rc.getContext('2d'), bx0 = d.Tx + W;
  rg.fillStyle = L.card2;
  rg.fillRect(0, 0, 0.24 * k, rc.height); rg.fillRect(rc.width - 0.24 * k, 0, 0.24 * k, rc.height);
  rg.fillRect(0, rc.height - 0.24 * k, rc.width, 0.24 * k); rg.fillRect((bx0 - fx0) * k, 0, (fx1 - bx0) * k, 0.24 * k);
  rg.fillStyle = rgba(L.light, 0.55);
  rg.fillRect(0, rc.height - 0.24 * k, rc.width, Math.max(1, k / 45));
  rg.strokeStyle = L.ink; rg.lineWidth = Math.max(1, k / 70);
  rg.strokeRect(0.24 * k, 0.24 * k, rc.width - 0.48 * k, rc.height - 0.48 * k);
  if (!L.plain && !L.simple) { rg.save(); rg.setLineDash([0.07 * k, 0.05 * k]); rg.strokeStyle = rgba(L.ink, 0.4); rg.lineWidth = Math.max(1, k / 100); rg.strokeRect(0.12 * k, 0.12 * k, rc.width - 0.24 * k, rc.height - 0.24 * k); rg.restore();
  // Mitred corners where the rim's strips meet.
  rg.strokeStyle = rgba(L.ink, 0.5); rg.beginPath(); rg.moveTo(0, rc.height); rg.lineTo(0.24 * k, rc.height - 0.24 * k); rg.moveTo(rc.width, rc.height); rg.lineTo(rc.width - 0.24 * k, rc.height - 0.24 * k); rg.stroke(); }
  grain(rc, L.grain, 0, 8);
  out.rims = { c: rc, x0: fx0, x1: fx1, z0: rz0, z1: fz1, y: tr.wall };
  var lc = cv((fx1 - fx0) * k, tr.wall * k), lg = lc.getContext('2d');
  lg.fillStyle = mix(L.card2, L.edge, 0.35); lg.fillRect(0, 0, lc.width, lc.height);
  lg.fillStyle = rgba(L.light, 0.5); lg.fillRect(0, 0, lc.width, Math.max(1, k / 45));
  lg.strokeStyle = L.ink; lg.lineWidth = Math.max(1, k / 70); lg.strokeRect(0.5, 0.5, lc.width - 1, lc.height - 1);
  if (L.ornate) { lg.strokeStyle = L.accent; lg.setLineDash([0.08 * k, 0.06 * k]); lg.beginPath(); lg.moveTo(0, lc.height / 2); lg.lineTo(lc.width, lc.height / 2); lg.stroke(); }
  grain(lc, L.grain, 0, 10);
  out.lip = { c: lc, x0: fx0, x1: fx1, y0: 0, y1: tr.wall, z: fz1 };
  var bc = cv((fx1 - bx0) * k, tr.wall * k), bg = bc.getContext('2d');
  bg.fillStyle = mix(L.card2, L.edge, 0.15); bg.fillRect(0, 0, bc.width, bc.height);
  bg.fillStyle = rgba('#000000', 0.12); bg.fillRect(0, bc.height * 0.6, bc.width, bc.height * 0.4);
  out.back = { c: bc, x0: bx0, x1: fx1, y0: 0, y1: tr.wall, z: tr.z0 };
  return out;
}

function floorFill(g, L, w, h) {
  if (L.floor === 'felt') { g.fillStyle = L.felt; g.fillRect(0, 0, w, h); }
  else {
    g.fillStyle = L.base; g.fillRect(0, 0, w, h);
    if (L.kind === 'paper') { g.strokeStyle = rgba(L.edge, 0.22); g.lineWidth = 1; var s = Math.max(12, w / 28); for (var x = s; x < w; x += s) { g.beginPath(); g.moveTo(x, 0); g.lineTo(x, h); g.stroke(); } for (var y = s; y < h; y += s) { g.beginPath(); g.moveTo(0, y); g.lineTo(w, y); g.stroke(); } }
  }
  if (L.simple) return;
  var v = g.createRadialGradient(w / 2, h / 2, Math.min(w, h) * 0.2, w / 2, h / 2, Math.max(w, h) * 0.7);
  v.addColorStop(0, 'rgba(0,0,0,0)'); v.addColorStop(1, 'rgba(0,0,0,' + (L.dark ? 0.25 : 0.08) + ')');
  g.fillStyle = v; g.fillRect(0, 0, w, h);
}

// ---------- The bowl, seen from straight above ----------
// R: where the physics wall stands. The floor is drawn to it; the sloping wall and the rim lie outside it.
function bowlArt(L, R, k, S) {
  var c = cv(S * k, S * k), g = c.getContext('2d'), m = c.width / 2, rf = R * k, rw = (R + 0.62) * k, ro = (R + 0.92) * k, N = 16;
  // Shadow on the table.
  g.save(); shadowOn(g, L, k, 2.2); g.beginPath(); g.arc(m, m, ro, 0, 6.3); g.fillStyle = L.edge; g.fill(); g.restore();
  // Rim: paper is a pleated rim, a kit a plain band.
  g.beginPath(); g.arc(m, m, ro, 0, 6.3); g.fillStyle = L.card; g.fill();
  if (L.kind === 'paper') {
    for (var i = 0; i < N; i++) {
      var a0 = i / N * 6.283, a1 = (i + 1) / N * 6.283, am = (a0 + a1) / 2;
      g.beginPath(); g.moveTo(m + Math.cos(a0) * rw, m + Math.sin(a0) * rw); g.lineTo(m + Math.cos(am) * ro * 1.004, m + Math.sin(am) * ro * 1.004); g.lineTo(m + Math.cos(a1) * rw, m + Math.sin(a1) * rw); g.closePath();
      g.fillStyle = i % 2 ? rgba(L.light, 0.6) : rgba(L.edge, 0.25); g.fill();
    }
  }
  // Inner wall: pleats (paper) or a smooth slope, lit from the top left.
  var wallG = g.createRadialGradient(m - 0.5 * k, m - 0.6 * k, rf * 0.9, m, m, rw);
  wallG.addColorStop(0, mix(L.card2, '#000000', 0.25)); wallG.addColorStop(1, L.card2);
  g.beginPath(); g.arc(m, m, rw, 0, 6.3); g.fillStyle = L.simple ? L.card2 : wallG; g.fill();
  if (L.kind === 'paper') {
    for (var j = 0; j < N; j++) {
      var b0 = j / N * 6.283, b1 = (j + 1) / N * 6.283, lit = Math.cos((b0 + b1) / 2 + 2.3);
      g.beginPath(); g.moveTo(m + Math.cos(b0) * rf, m + Math.sin(b0) * rf); g.lineTo(m + Math.cos(b0) * rw, m + Math.sin(b0) * rw); g.arc(m, m, rw, b0, b1); g.lineTo(m + Math.cos(b1) * rf, m + Math.sin(b1) * rf); g.arc(m, m, rf, b1, b0, true); g.closePath();
      g.fillStyle = lit > 0 ? rgba(L.light, 0.35 * lit) : rgba('#000000', -0.18 * lit); g.fill();
      g.strokeStyle = rgba(L.ink, 0.3); g.lineWidth = 1; g.beginPath(); g.moveTo(m + Math.cos(b0) * rf, m + Math.sin(b0) * rf); g.lineTo(m + Math.cos(b0) * rw, m + Math.sin(b0) * rw); g.stroke();
    }
  } else if (!L.simple) {
    var sh = g.createLinearGradient(m - rw, m - rw, m + rw, m + rw); sh.addColorStop(0, rgba('#000000', 0.18)); sh.addColorStop(0.5, 'rgba(0,0,0,0)'); sh.addColorStop(1, rgba(L.light, 0.3));
    g.beginPath(); g.arc(m, m, rw, 0, 6.3); g.arc(m, m, rf, 0, 6.3, true); g.fillStyle = sh; g.fill();
    if (L.sketch) { g.beginPath(); g.arc(m, m, rw, 0, 6.3); g.arc(m, m, rf, 0, 6.3, true); hatch(g, L, k, 0, 0, c.width, c.height); }
  }
  // Floor, with the wall's shadow falling on its top-left edge.
  g.save(); g.beginPath(); g.arc(m, m, rf, 0, 6.3); g.clip();
  floorFill(g, L, c.width, c.height);
  var fs = g.createRadialGradient(m + 0.25 * k, m + 0.3 * k, rf * 0.75, m + 0.12 * k, m + 0.15 * k, rf * 1.05);
  fs.addColorStop(0, 'rgba(0,0,0,0)'); fs.addColorStop(1, L.shadow);
  if (!L.simple) { g.fillStyle = fs; g.fillRect(0, 0, c.width, c.height); } g.restore();
  // Outlines.
  g.beginPath(); g.arc(m, m, ro, 0, 6.3); ink(g, L, k, L.sharp ? 2 : 1.3);
  g.beginPath(); g.arc(m, m, rw, 0, 6.3); ink(g, L, k);
  g.beginPath(); g.arc(m, m, rf, 0, 6.3); ink(g, L, k, 0.8);
  if (L.ornate) { g.beginPath(); g.arc(m, m, (ro + rw) / 2, 0, 6.3); g.setLineDash([0.06 * k, 0.06 * k]); g.strokeStyle = L.accent; g.lineWidth = Math.max(1, k / 50); g.stroke(); g.setLineDash([]); }
  if (L.lumen) { g.save(); g.beginPath(); g.arc(m, m, rw, 0, 6.3); g.shadowColor = L.accent; g.shadowBlur = 0.25 * k; g.strokeStyle = rgba(L.accent, 0.7); g.lineWidth = Math.max(1.5, k / 40); g.stroke(); g.restore(); }
  grain(c, L.floor === 'felt' ? 14 : L.grain, L.fibres, 11);
  return c;
}

// ---------- The table mat, seen from straight above ----------
// hw, hd: half the play area, where the unseen walls stand; B: the border drawn outside it.
// Simple is one flat token fill with a hairline; Flat paper a printed card mat; Detailed a cloth mat
// with a stitched border, a faint grid and a soft shadow under it.
var MAT_B = 0.5;
function matArt(L, hw, hd, k) {
  var B = MAT_B, P = 0.25, W = (hw + B) * 2, H = (hd + B) * 2, c = cv((W + P * 2) * k, (H + P * 2) * k), g = c.getContext('2d');
  var x0 = P * k, y0 = P * k, w = W * k, h = H * k, b = B * k, hl = Math.max(1, k / 80);
  function grid(col, step) {
    g.save(); g.beginPath(); g.rect(x0 + b, y0 + b, w - 2 * b, h - 2 * b); g.clip(); g.strokeStyle = col; g.lineWidth = hl;
    for (var x = x0 + w / 2 % (step * k); x < x0 + w; x += step * k) { g.beginPath(); g.moveTo(x, y0); g.lineTo(x, y0 + h); g.stroke(); }
    for (var y = y0 + h / 2 % (step * k); y < y0 + h; y += step * k) { g.beginPath(); g.moveTo(x0, y); g.lineTo(x0 + w, y); g.stroke(); }
    g.restore();
  }
  if (L.simple) {
    var sr = Math.max(0.04, L.r) * k * 1.5;
    g.save(); g.shadowColor = L.shadow; g.shadowBlur = 0.1 * k; g.shadowOffsetY = 0.03 * k;
    g.beginPath(); rr(g, x0, y0, w, h, sr); g.fillStyle = L.floor === 'felt' ? L.felt : L.card2; g.fill(); g.restore();
    g.beginPath(); rr(g, x0 + 0.5, y0 + 0.5, w - 1, h - 1, sr); g.lineWidth = hl; g.strokeStyle = L.edge; g.stroke();
    return c;
  }
  if (L.plain) {
    // Printed card: paper stock, a ruled double border with corner squares, a faint printed grid.
    g.beginPath(); rr(g, x0, y0, w, h, L.r * k); g.fillStyle = L.card; g.fill(); ink(g, L, k, 1.3);
    g.beginPath(); g.rect(x0 + b * 0.25, y0 + b * 0.25, w - b * 0.5, h - b * 0.5); g.rect(x0 + b, y0 + b, w - 2 * b, h - 2 * b);
    g.fillStyle = L.card2; g.fill('evenodd');
    g.beginPath(); g.rect(x0 + b * 0.25, y0 + b * 0.25, w - b * 0.5, h - b * 0.5); ink(g, L, k, 1);
    g.beginPath(); g.rect(x0 + b, y0 + b, w - 2 * b, h - 2 * b); ink(g, L, k, 1);
    [[x0, y0], [x0 + w - b, y0], [x0, y0 + h - b], [x0 + w - b, y0 + h - b]].forEach(function (p) { g.beginPath(); g.rect(p[0] + b * 0.25, p[1] + b * 0.25, b * 0.75, b * 0.75); g.fillStyle = L.card; g.fill(); ink(g, L, k, 1); });
    grid(rgba(L.edge, 0.45), 1);
    return c;
  }
  // Cloth: the felt tone, a darker border band, a stitched seam, a faint grid, and shading toward the edge.
  var cloth = L.felt, band = mix(cloth, '#000000', 0.28), stitch = L.kind === 'kit' ? L.accent : mix(L.light || '#ffffff', cloth, 0.25);
  g.save(); shadowOn(g, L, k, 2); g.beginPath(); rr(g, x0, y0, w, h, Math.max(0.08, L.r) * k); g.fillStyle = band; g.fill(); g.restore();
  g.beginPath(); rr(g, x0 + b * 0.8, y0 + b * 0.8, w - b * 1.6, h - b * 1.6, Math.max(0.04, L.r * 0.6) * k); g.fillStyle = cloth; g.fill();
  grid('rgba(255,255,255,.07)', 1);
  var v = g.createRadialGradient(x0 + w / 2, y0 + h / 2, Math.min(w, h) * 0.25, x0 + w / 2, y0 + h / 2, Math.max(w, h) * 0.62);
  v.addColorStop(0, 'rgba(0,0,0,0)'); v.addColorStop(1, 'rgba(0,0,0,.28)');
  g.save(); g.beginPath(); rr(g, x0 + b * 0.8, y0 + b * 0.8, w - b * 1.6, h - b * 1.6, Math.max(0.04, L.r * 0.6) * k); g.clip(); g.fillStyle = v; g.fillRect(x0, y0, w, h);
  g.strokeStyle = 'rgba(0,0,0,.35)'; g.lineWidth = 0.08 * k; g.stroke(); g.restore();
  g.save(); g.setLineDash([0.12 * k, 0.08 * k]); g.lineWidth = Math.max(1.5, k / 45); g.strokeStyle = stitch;
  g.beginPath(); rr(g, x0 + b * 0.4, y0 + b * 0.4, w - b * 0.8, h - b * 0.8, Math.max(0.06, L.r) * k); g.stroke(); g.restore();
  if (L.ornate) [[x0 + b * 0.4, y0 + b * 0.4], [x0 + w - b * 0.4, y0 + b * 0.4], [x0 + b * 0.4, y0 + h - b * 0.4], [x0 + w - b * 0.4, y0 + h - b * 0.4]].forEach(function (p) { g.beginPath(); g.arc(p[0], p[1], 0.09 * k, 0, 6.3); g.fillStyle = L.accent; g.fill(); });
  grain(c, 16, 0, 13);
  return c;
}

  W.Chronicle.diceTrayArt = { mix: mix, KIT_R: KIT_R, lookFrom: lookFrom, CAP: CAP, MER: MER, OVER: OVER, MAT_B: MAT_B, towerShape: towerShape, towerArt: towerArt, bowlArt: bowlArt, matArt: matArt };
})();
