// ---------- Crit fail: Shatter ----------
// The 1 flickers a sick colour while hairline cracks race over the die (drawn on the faces' light
// layers) and a sharp crack splits it. It bursts into its face pieces, fine chips and glitter, and
// time stops: the pieces drift up toward the viewer, turn to face it and hang, slowly turning,
// inside a ring of stopped clock ticks, casting shadows on the page. Then time snaps back: the ring
// closes, the pieces rush in and slam together, a shockwave runs over the page and a few chips that
// were lost tinkle away. The die sits whole with faint scars that fade and the 1 lit cold red.
import {
  clamp01, cutPaths, drop, faceToLocal, glow, glowFace, glowStrokes, hexA, in3, isLight, mix,
  mulberry, numberOnto, out3, paintNumeral, polyLen, registerCrit, shadeDie, smooth, splitDie,
  spriteTex, starTex, strokePaths, strokeW, tintDie, wholeDie, worldOf
} from '../engine.js';

var SHAT_SICK = '#b4e832', SHAT_SICK_DEEP = '#5f8400', SHAT_CRACK = '#d9ff86', SHAT_COLD = '#d6ecff', SHAT_RED = '#ff3a5c', SHAT_RED_DEEP = '#c4123a', SHAT_INK_SICK = '#9ccf10';

// A pale stone has nothing for added light to stand on: the number is drawn in a deeper solid colour.
function shatterPale(th) { try { return !!isLight(th); } catch (err) { return false; } }

// The 1 stutters: a fast random gate over a quick shiver, like a failing lamp.
function shatterFlick(e, lo) {
  var h = Math.sin(Math.floor(e * 21) * 12.9898 + 4.1) * 43758.5453, h2 = Math.sin(Math.floor(e * 9) * 78.233) * 12543.1;
  h -= Math.floor(h); h2 -= Math.floor(h2);
  return (h < 0.24 ? lo : 1) * (h2 < 0.12 ? lo : 1) * (0.82 + 0.18 * Math.sin(e * 61));
}

// Places the grand keeps clear, as plain { x, z, r }. Anything of ours that hangs, drifts or darkens the
// page stays out of them, so another crit's number is never covered.
function shatterZones(o) { return ((o && o.clear) || []).map(function (z) { return { x: z.x, z: z.z, r: z.r }; }); }
function shatterIn(x, z, zones, pad) {
  for (var i = 0; i < zones.length; i++) if (Math.hypot(x - zones[i].x, z - zones[i].z) < zones[i].r + pad) return true;
  return false;
}
// Move p (x, z) out of every zone and back inside the canvas, edge units from its rim.
function shatterPush(st, p, zones, pad, edge) {
  for (var pass = 0; pass < 3; pass++) {
    for (var i = 0; i < zones.length; i++) {
      var z = zones[i], dx = p.x - z.x, dz = p.z - z.z, d = Math.hypot(dx, dz), need = z.r + pad;
      if (d < need) { if (d < 1e-3) { dx = 1; dz = 0; d = 1; } p.x = z.x + dx / d * need; p.z = z.z + dz / d * need; }
    }
    p.x = Math.max(-(st.hw - edge), Math.min(st.hw - edge, p.x)); p.z = Math.max(-(st.hh - edge), Math.min(st.hh - edge, p.z));
  }
  return p;
}
// The way the key light throws shadows across the page, per unit of height.
function shatterThrow(st) { var sp = st.sun.position; return { x: -sp.x / sp.y, z: -sp.z / sp.y }; }

// The number as light on a face's glow layer. On a pale stone it is a deeper solid colour.
function shatterNumeral(f, color, solid) {
  var g2 = f.g, lay = f.lay;
  if (solid && lay.style === 'classic') {
    g2.shadowColor = color; g2.shadowBlur = lay.h * 0.16; g2.fillStyle = color; paintNumeral(g2, lay, 'classic'); g2.shadowBlur = 0; paintNumeral(g2, lay, 'classic');
  } else if (solid) {
    var w = strokeW(lay);
    g2.lineJoin = 'round'; g2.lineCap = 'round'; g2.shadowColor = color; g2.shadowBlur = w; g2.strokeStyle = color; g2.lineWidth = w * 1.25;
    strokePaths(g2, lay.strokes);
    g2.shadowBlur = 0;
  } else numberOnto(g2, lay, color);
}

// A pale stone has nothing for added light to stand on, so there the light layers are drawn as ink: blended
// normally, transparent where nothing is drawn, their opacity the level. Put back to light when done.
function shatterInkMode(T, fcs, on) {
  fcs.forEach(function (fc) { if (fc.over) { fc.over.material.blending = on ? T.NormalBlending : T.CustomBlending; fc.over.material.opacity = 1; } });
}
function shatterLevel(fc, k, ink) {
  if (!ink) { fc.mat.emissiveIntensity = k; return; }
  fc.mat.emissiveIntensity = k > 0.001 ? 1 : 0; fc.over.material.opacity = clamp01(k / 1.6);
}
// A crack as ink: a soft halo of its colour round a dark core.
function shatterInkStrokes(g, paths, w, halo) {
  g.lineJoin = 'round'; g.lineCap = 'round';
  g.strokeStyle = hexA(halo, 0.4); g.lineWidth = w * 2.3; strokePaths(g, paths);
  g.strokeStyle = 'rgba(18,5,14,0.96)'; g.lineWidth = w * 1.5; strokePaths(g, paths);
}

// ---------- The cracks ----------
// A jagged line from A to B in canvas px. The ends are exact (so a crack meets its neighbour on the
// shared rim); the kinks keep their side a while, as real cracks do, and settle toward the ends.
function shatterJag(A, B, rnd, amp, step) {
  var dx = B[0] - A[0], dy = B[1] - A[1], L = Math.hypot(dx, dy) || 1, n = Math.max(3, Math.round(L / step)), nx = -dy / L, ny = dx / L, out = [[A[0], A[1]]], sg = rnd() < 0.5 ? 1 : -1;
  for (var i = 1; i < n; i++) {
    var u = i / n + (rnd() - 0.5) * 0.3 / n, off = sg * amp * (0.3 + rnd() * 0.7) * Math.sqrt(Math.sin(u * Math.PI));
    out.push([A[0] + dx * u + nx * off, A[1] + dy * u + ny * off]);
    if (rnd() < 0.62) sg = -sg;
  }
  out.push([B[0], B[1]]);
  return out;
}
// A fine side crack off point j of a line.
function shatterBranch(pl, j, rnd, len, amp) {
  var a = pl[Math.max(0, j - 1)], b = pl[Math.min(pl.length - 1, j + 1)], ang = Math.atan2(b[1] - a[1], b[0] - a[0]) + (rnd() < 0.5 ? -1 : 1) * (0.5 + rnd() * 0.6);
  var p = pl[j], out = [[p[0], p[1]]], n = 2 + Math.floor(rnd() * 2), x = p[0], y = p[1];
  for (var i = 0; i < n; i++) { ang += (rnd() - 0.5) * 0.7; x += Math.cos(ang) * len / n; y += Math.sin(ang) * len / n; out.push([x, y]); }
  return out;
}
// The crack network over a whole die. Cracks cross each rim at points shared by both faces, so one
// crack runs on from face to face; the front starts on the 1's face and spreads by faces away from
// it. Each path has a time window in "face steps" (t0 to t1) in which it is drawn, from its start.
function shatterNet(die, fi, seed) {
  var F = die.faces, rnd = mulberry(seed), edges = {}, V = die.sd.v, tmax = 0, maxD = 0;
  function key(a, b) { return Math.min(a, b) + '|' + Math.max(a, b); }
  F.forEach(function (fc, f) {
    var idx = fc.f.idx;
    for (var k = 0; k < idx.length; k++) {
      var a = idx[k], b = idx[(k + 1) % idx.length], kk = key(a, b);
      if (!edges[kk]) edges[kk] = { a: Math.min(a, b), b: Math.max(a, b), faces: [], on: rnd() < 0.62, u: 0.27 + rnd() * 0.46 };
      edges[kk].faces.push(f);
    }
  });
  var fe = F.map(function (fc) { var idx = fc.f.idx; return idx.map(function (a, k) { return edges[key(a, idx[(k + 1) % idx.length])]; }); });
  // Every face carries at least two cracks, so none is left whole.
  fe.forEach(function (list) {
    var on = list.filter(function (x) { return x.on; }).length, guard = 0;
    while (on < Math.min(2, list.length) && guard++ < 12) { var pk = list[Math.floor(rnd() * list.length)]; if (!pk.on) { pk.on = true; on++; } }
  });
  var depth = F.map(function () { return -1; }), queue = [fi]; depth[fi] = 0;
  while (queue.length) {
    var f0 = queue.shift();
    fe[f0].forEach(function (ed) { if (ed.on) ed.faces.forEach(function (g) { if (g !== f0 && depth[g] < 0) { depth[g] = depth[f0] + 1; queue.push(g); } }); });
  }
  // A face the cracks cannot reach joins the nearest one that is reached.
  for (var again = 0, changed = true; changed && again < 8; again++) {
    changed = false;
    F.forEach(function (fc, f) {
      if (depth[f] >= 0) return;
      fe[f].forEach(function (ed) { ed.faces.forEach(function (g) { if (g !== f && depth[g] >= 0 && depth[f] < 0) { ed.on = true; depth[f] = depth[g] + 1; changed = true; } }); });
    });
  }
  depth.forEach(function (d) { maxD = Math.max(maxD, d); });
  var faces = F.map(function (fc, f) {
    var lay = fc.lay, ff = fc.f, cv = lay.cv, rr = ff.inr * (f === fi ? 0.1 : 0.5), a0 = rnd() * 6.2832, rd = rr * Math.sqrt(rnd());
    var hubP = { x: ff.c.x + ff.right.x * Math.cos(a0) * rd + ff.up.x * Math.sin(a0) * rd, y: ff.c.y + ff.right.y * Math.cos(a0) * rd + ff.up.y * Math.sin(a0) * rd, z: ff.c.z + ff.right.z * Math.cos(a0) * rd + ff.up.z * Math.sin(a0) * rd };
    var hub = lay.px(hubP), paths = [], tS = 99, tE = 0;
    function add(pl, t0, t1) { paths.push({ pl: pl, t0: t0, t1: t1 }); tS = Math.min(tS, t0); tE = Math.max(tE, t1); }
    fe[f].forEach(function (ed) {
      if (!ed.on) return;
      var other = ed.faces[0] === f ? ed.faces[1] : ed.faces[0];
      var src = (other == null || depth[f] < depth[other] || (depth[f] === depth[other] && f < other)) ? f : other, isSrc = src === f, ds = Math.max(0, depth[src]);
      var va = V[ed.a], vb = V[ed.b], port = lay.px({ x: va.x + (vb.x - va.x) * ed.u, y: va.y + (vb.y - va.y) * ed.u, z: va.z + (vb.z - va.z) * ed.u });
      var A = isSrc ? hub : port, B = isSrc ? port : hub, t0 = isSrc ? ds : ds + 0.5, pl = shatterJag(A, B, rnd, cv * 0.026, cv * 0.055);
      add(pl, t0, t0 + 0.5);
      if (rnd() < 0.8) { var j = Math.max(1, Math.min(pl.length - 2, Math.floor(pl.length * (0.3 + rnd() * 0.4)))), u = j / (pl.length - 1); add(shatterBranch(pl, j, rnd, cv * (0.07 + rnd() * 0.08), cv * 0.02), t0 + 0.5 * u, t0 + 0.5 * u + 0.32); }
    });
    if (f === fi) for (var r = 0; r < 5; r++) {
      var an = r / 5 * 6.2832 + rnd() * 0.8, Lr = cv * (0.07 + rnd() * 0.1);
      add(shatterJag(hub, [hub[0] + Math.cos(an) * Lr, hub[1] + Math.sin(an) * Lr], rnd, cv * 0.012, cv * 0.04), 0, 0.3);
    }
    return { paths: paths, tS: tS, tE: tE };
  });
  faces.forEach(function (fn) { tmax = Math.max(tmax, fn.tE); });
  return { faces: faces, tmax: tmax, depth: depth };
}
// The part of a face's cracks drawn by step G.
function shatterCracksAt(fn, G) {
  var out = [];
  for (var i = 0; i < fn.paths.length; i++) {
    var pt = fn.paths[i], p = (G - pt.t0) / (pt.t1 - pt.t0);
    if (p <= 0) continue;
    if (p >= 1) out.push(pt.pl); else { var cp = cutPaths([pt.pl], p); if (cp.length) out.push(cp[0]); }
  }
  return out;
}
// A path from fraction a to b of its length (a zipper closing a crack).
function shatterSlice(pl, a, b) {
  var L = polyLen(pl), from = L * a, to = L * b, run = 0, out = [];
  for (var k = 1; k < pl.length; k++) {
    var sl = Math.hypot(pl[k][0] - pl[k - 1][0], pl[k][1] - pl[k - 1][1]), s0 = run, s1 = run + sl; run = s1;
    if (s1 <= from || s0 >= to) continue;
    var f0 = Math.max(0, (from - s0) / sl), f1 = Math.min(1, (to - s0) / sl);
    if (!out.length) out.push([pl[k - 1][0] + (pl[k][0] - pl[k - 1][0]) * f0, pl[k - 1][1] + (pl[k][1] - pl[k - 1][1]) * f0]);
    out.push([pl[k - 1][0] + (pl[k][0] - pl[k - 1][0]) * f1, pl[k - 1][1] + (pl[k][1] - pl[k - 1][1]) * f1]);
  }
  return out;
}

// ---------- Fissures on the page ----------
// Ribbons lying on the page: lines is [{ pts: [[x, z]...], d0, w }]. aD is the distance along each
// line (from the die, plus d0), which a shader reveals up to a reach, so the same geometry races out,
// holds, closes again or zips shut.
function shatterRibbonGeo(T, lines, y) {
  var pos = [], aD = [], aS = [], idx = [];
  lines.forEach(function (ln) {
    var P = ln.pts, base = pos.length / 3, d = ln.d0 || 0;
    for (var i = 0; i < P.length; i++) {
      var a = P[Math.max(0, i - 1)], b = P[Math.min(P.length - 1, i + 1)], tx = b[0] - a[0], tz = b[1] - a[1], tl = Math.hypot(tx, tz) || 1;
      if (i) d += Math.hypot(P[i][0] - P[i - 1][0], P[i][1] - P[i - 1][1]);
      var w = ln.w * (1 - 0.6 * i / (P.length - 1)), nx = -tz / tl * w, nz = tx / tl * w;
      pos.push(P[i][0] + nx, y, P[i][1] + nz, P[i][0] - nx, y, P[i][1] - nz); aD.push(d, d); aS.push(-1, 1);
      if (i) { var q = base + (i - 1) * 2; idx.push(q, q + 1, q + 2, q + 1, q + 3, q + 2); }
    }
  });
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('aD', new T.Float32BufferAttribute(aD, 1)); geo.setAttribute('aS', new T.Float32BufferAttribute(aS, 1));
  geo.setIndex(idx);
  return geo;
}
// A fissure: a dark spine in a glow of the crack's colour, hottest at its growing tip and, when it
// closes, at the seam that is closing. Dark in the middle so it shows on a white page, lit round it so
// it shows on a dark one.
function shatterFissureMat(st, color) {
  var T = st.T, uni = { uReach: { value: 0 }, uTail: { value: -1 }, uFade: { value: 1 }, uHot: { value: 1 }, uSeal: { value: 0 }, uGlow: { value: new T.Color(color) }, uBoost: { value: st.cine ? 1 : 1.5 } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false, side: T.DoubleSide,
    vertexShader: 'attribute float aD; attribute float aS; varying float vD; varying float vS; void main(){ vD = aD; vS = aS; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: ['uniform float uReach, uTail, uFade, uHot, uSeal, uBoost; uniform vec3 uGlow; varying float vD; varying float vS;',
      'void main(){',
      ' if (vD > uReach || vD < uTail) discard;',
      ' float c = 1.0 - abs(vS);',
      ' float hot = clamp(exp(-(uReach - vD) * 3.0) * uHot + exp(-(vD - uTail) * 5.0) * uSeal, 0.0, 1.0);',
      ' float halo = pow(c, 1.5), spine = smoothstep(0.7, 0.92, c);',
      ' vec3 col = mix(uGlow * (0.8 + hot * 1.4), vec3(0.03, 0.02, 0.05), spine * (1.0 - hot * 0.85));',
      ' col = mix(col, vec3(1.0), hot * smoothstep(0.55, 1.0, c) * 0.6);',
      ' float a = clamp(halo * 0.7 + spine * 0.4, 0.0, 1.0) * uFade;',
      ' gl_FragColor = vec4(col, 1.0 - pow(1.0 - a, uBoost));',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
  mat.userData.uni = uni;
  return mat;
}
// The cracks that run out over the page from the die: jagged lines that stay clear of the zones and the
// canvas rim, each with a branch or two.
function shatterFissureLines(st, c, s, hd, zones, rnd) {
  var N = hd ? 9 : 6, lines = [], reach = 0;
  for (var i = 0; i < N; i++) {
    var a = i / N * 6.2832 + (rnd() - 0.5) * 0.6, L = s * (2.3 + rnd() * 2.5), step = s * 0.32, r0 = s * 0.7, x = c.x + Math.cos(a) * r0, z = c.z + Math.sin(a) * r0, d = r0, ang = a, pts = [[x, z]], br = 0;
    while (d < L) {
      ang += (rnd() - 0.5) * 0.6; x += Math.cos(ang) * step; z += Math.sin(ang) * step; d += step;
      if (shatterIn(x, z, zones, s * 0.45) || Math.abs(x) > st.hw - 0.3 || Math.abs(z) > st.hh - 0.3) break;
      pts.push([x, z]);
      if (br < (hd ? 2 : 1) && d > L * 0.35 && rnd() < 0.3) {
        br++; var ba = ang + (rnd() < 0.5 ? -1 : 1) * (0.5 + rnd() * 0.5), bx = x, bz = z, bd = d, bp = [[x, z]];
        for (var k = 0; k < 3; k++) { ba += (rnd() - 0.5) * 0.6; bx += Math.cos(ba) * step * 0.8; bz += Math.sin(ba) * step * 0.8; bd += step * 0.8; if (shatterIn(bx, bz, zones, s * 0.45) || Math.abs(bx) > st.hw - 0.3 || Math.abs(bz) > st.hh - 0.3) break; bp.push([bx, bz]); }
        if (bp.length > 1) { lines.push({ pts: bp, d0: d, w: 0.035 * s / 1.25 }); reach = Math.max(reach, bd); }
      }
    }
    if (pts.length > 2) { lines.push({ pts: pts, d0: r0, w: 0.05 * s / 1.25 }); reach = Math.max(reach, d); }
  }
  return { lines: lines, reach: reach };
}

// ---------- The page under it: a cold dim, and the ring of stopped clock ticks ----------
// One quad: a soft shade over the middle (so the glow reads on a white page) and a ring of ticks with a
// shimmering band, light and dark together so it shows on either page. Holes round the zones keep both
// off the other dice.
function shatterPageMat(st, ks, zones, at, Rd) {
  var T = st.T, hole = [], near = zones.slice().sort(function (a, b) { return Math.hypot(a.x - at.x, a.z - at.z) - Math.hypot(b.x - at.x, b.z - at.z); }).slice(0, 6);
  for (var i = 0; i < 6; i++) hole.push(near[i] ? new T.Vector3(near[i].x - at.x, -(near[i].z - at.z), near[i].r) : new T.Vector3(0, 0, 0));
  var uni = { uDim: { value: 0 }, uR: { value: 0 }, uRot: { value: 0 }, uA: { value: 0 }, uTime: { value: 0 }, uK: { value: ks }, uRd: { value: Rd }, uFlash: { value: 0 }, uShockR: { value: 0 }, uShockA: { value: 0 }, uHole: { value: hole },
    uLight: { value: new T.Color('#cfe6ff') }, uDark: { value: new T.Color('#1a2236') } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false,
    vertexShader: 'varying vec2 vP; void main(){ vP = position.xy; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: ['uniform float uDim, uR, uRot, uA, uTime, uK, uRd, uFlash, uShockR, uShockA; uniform vec3 uHole[6]; uniform vec3 uLight, uDark; varying vec2 vP;',
      'float holes(vec2 p){ float m = 1.0; for (int i = 0; i < 6; i++) { vec3 h = uHole[i]; if (h.z > 0.0) m *= smoothstep(h.z - 0.2, h.z + 0.5, length(p - h.xy)); } return m; }',
      'void main(){',
      ' float r = length(vP), a = atan(vP.y, vP.x) + uRot, hm = holes(vP);',
      ' float dimA = uDim * smoothstep(uRd, uRd * 0.3, r) * hm;',
      ' float w = 0.026 * uK, d1 = abs(r - uR), d2 = abs(r - (uR - 0.22 * uK));',
      ' float l1 = smoothstep(w, 0.0, d1), l2 = smoothstep(w * 0.8, 0.0, d2);',
      // Ticks every 5 degrees between the two lines, every sixth one long.
      ' float x = a / 6.2831853 * 72.0, cell = floor(x), fr = fract(x), dd = min(fr, 1.0 - fr), isLong = step(0.5, 1.0 - mod(cell, 6.0));',
      ' float tw = 0.02 * uK / max(6.2831853 * max(r, 0.01) / 72.0, 0.001);',
      ' float inner = uR - (0.22 + 0.2 * isLong) * uK, span = smoothstep(inner - 0.01, inner + 0.02, r) * smoothstep(uR + 0.01, uR - 0.02, r);',
      ' float tick = smoothstep(tw, tw * 0.3, dd) * span;',
      ' float band = exp(-pow((r - uR + 0.1 * uK) / (0.5 * uK), 2.0)) * (0.55 + 0.45 * sin(r * 26.0 / uK - uTime * 1.6 + a * 3.0));',
      ' float lightR = clamp(l1 + l2 * 0.7 + tick * 0.9 + band * 0.16 + uFlash * l1, 0.0, 1.0) * step(0.001, uR) * uA;',
      ' float darkR = clamp(smoothstep(w * 2.6, 0.0, d1) * 0.6 + tick * 0.5 + band * 0.12, 0.0, 1.0) * step(0.001, uR) * uA;',
      // A shock ring: a thin bright edge with a soft trail behind it, and a dark band under it, so it
      // shows on a white page as well as a dark one.
      ' float sd = r - uShockR, on = step(0.001, uShockR) * uShockA;',
      ' float sL = (exp(-pow(sd / (0.09 * uK), 2.0)) + 0.28 * smoothstep(-0.9 * uK, 0.0, sd) * smoothstep(0.1 * uK, 0.0, sd)) * on;',
      ' float sD = exp(-pow((sd + 0.1 * uK) / (0.2 * uK), 2.0)) * 0.6 * on;',
      ' float light = clamp(lightR + sL, 0.0, 1.0), dark = clamp(darkR + sD, 0.0, 1.0);',
      ' float ra = max(dark * 0.5, light * 0.9) * hm;',
      ' vec3 rc = mix(uDark, uLight, light / max(light + dark * 0.5, 0.001));',
      ' vec3 dc = vec3(0.02, 0.03, 0.08);',
      ' float oa = ra + dimA * (1.0 - ra);',
      ' if (oa < 0.002) discard;',
      ' vec3 col = (rc * ra + dc * dimA * (1.0 - ra)) / oa;',
      ' gl_FragColor = vec4(col, oa);',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
  mat.userData.uni = uni;
  return mat;
}

// ---------- Dust, glitter, chips and shadows ----------
// A pool of points that bursts are fired into, so many small bursts cost one draw call. update(dt, k,
// fade, pull): k slows the motion (time stopping) without ending the particles' lives, pull draws them in.
function shatterPool(st, n, o) {
  var T = st.T, pos = new Float32Array(n * 3), col = new Float32Array(n * 4), vel = new Float32Array(n * 3), age = new Float32Array(n), life = new Float32Array(n), ph = new Float32Array(n), cur = 0, rnd = Math.random;
  var cs = o.colors.map(function (h) { return new T.Color(h).multiplyScalar(o.boost || 1); });
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('color', new T.BufferAttribute(col, 4));
  var pts = new T.Points(geo, new T.PointsMaterial({ size: st.ptSize(o.size), map: o.tex, transparent: true, depthWrite: false, vertexColors: true, sizeAttenuation: true,
    blending: o.add ? T.AdditiveBlending : T.NormalBlending, toneMapped: !o.add }));
  pts.frustumCulled = false; pts.renderOrder = o.order; st.scene.add(pts);
  return { obj: pts,
    emit: function (k, fn) {
      for (var j = 0; j < k; j++) {
        var i = cur, sp = fn(j, rnd), c = cs[j % cs.length]; cur = (cur + 1) % n;
        pos[i * 3] = sp.p[0]; pos[i * 3 + 1] = sp.p[1]; pos[i * 3 + 2] = sp.p[2]; vel[i * 3] = sp.v[0]; vel[i * 3 + 1] = sp.v[1]; vel[i * 3 + 2] = sp.v[2];
        age[i] = -(sp.delay || 0); life[i] = sp.life; ph[i] = rnd() * 6.283;
        col[i * 4] = c.r; col[i * 4 + 1] = c.g; col[i * 4 + 2] = c.b; col[i * 4 + 3] = 0;
      }
    },
    update: function (dt, k, fade, pull) {
      var g = o.g || 0;
      for (var i = 0; i < n; i++) {
        if (life[i] <= 0) continue;
        age[i] += dt * (0.3 + 0.7 * k);
        if (age[i] < 0) { col[i * 4 + 3] = 0; continue; }
        var u = age[i] / life[i], j = i * 3;
        if (u >= 1) { col[i * 4 + 3] = 0; life[i] = 0; continue; }
        var h = dt * k;
        vel[j + 1] -= g * h;
        if (pull) { var pk = pull.k * dt; vel[j] += (pull.x - pos[j]) * pk; vel[j + 1] += (pull.y - pos[j + 1]) * pk; vel[j + 2] += (pull.z - pos[j + 2]) * pk; var dd = Math.max(0, 1 - 4 * dt); vel[j] *= dd; vel[j + 1] *= dd; vel[j + 2] *= dd; h = dt; }
        pos[j] += vel[j] * h; pos[j + 1] += vel[j + 1] * h; pos[j + 2] += vel[j + 2] * h;
        if (pos[j + 1] < 0.03) { pos[j + 1] = 0.03; vel[j + 1] *= -0.3; vel[j] *= 0.7; vel[j + 2] *= 0.7; }
        var a = o.alpha * fade * smooth(0, 0.1, u) * (1 - smooth(0.6, 1, u));
        // Glitter twinkles: each grain turns to catch the light now and then.
        if (o.twinkle) a *= 0.2 + 0.8 * Math.pow(0.5 + 0.5 * Math.sin(age[i] * o.twinkle + ph[i]), 3);
        col[i * 4 + 3] = a;
      }
      geo.attributes.position.needsUpdate = true; geo.attributes.color.needsUpdate = true;
    } };
}
// A chip of broken die: a thin, uneven sliver with sharp edges, base at the origin.
function shatterChipGeo(T) {
  var pos = [], top = [0.04, 0.13, 0.02], bot = [-0.03, -0.1, -0.02], rim = [[1, 0, 0.1], [0.1, 0.02, 0.95], [-0.8, -0.02, 0.2], [-0.2, 0.01, -0.85]];
  function tri(p, q, s) { pos.push(p[0], p[1], p[2], q[0], q[1], q[2], s[0], s[1], s[2]); }
  for (var k = 0; k < rim.length; k++) { var j = (k + 1) % rim.length; tri(rim[k], top, rim[j]); tri(rim[k], rim[j], bot); }
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.computeVertexNormals();
  return geo;
}
// Soft shadows on the page: each piece's own outline projected along the key light, as a core with a
// feathered rim, so a shadow has the shape of its tile. Tinted for glass, which throws coloured light
// and not a black shadow. The corner counts are fixed per piece, so one buffer serves them all.
function shatterShadowMesh(st, corners, tint) {
  var T = st.T, tris = 0, i;
  corners.forEach(function (m) { tris += m * 3; });
  var pos = new Float32Array(tris * 9), al = new Float32Array(tris * 3), geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('aAlpha', new T.BufferAttribute(al, 1));
  var mat = new T.ShaderMaterial({ transparent: true, depthWrite: false, side: T.DoubleSide, uniforms: { uCol: { value: new T.Color(tint) } },
    vertexShader: 'attribute float aAlpha; varying float vA; void main(){ vA = aAlpha; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: ['uniform vec3 uCol; varying float vA;', 'void main(){ if (vA < 0.002) discard; gl_FragColor = vec4(uCol, vA);', ' #include <tonemapping_fragment>', ' #include <colorspace_fragment>', '}'].join('\n') });
  var mesh = new T.Mesh(geo, mat); mesh.frustumCulled = false; mesh.renderOrder = -1; mesh.visible = false;
  var vi = 0, ai = 0;
  function vert(x, z, a) { pos[vi++] = x; pos[vi++] = 0.006; pos[vi++] = z; al[ai++] = a; }
  return { mesh: mesh, begin: function () { vi = 0; ai = 0; },
    // One piece: world corners in P (flat x, y, z), its alpha, and how soft its edge is.
    add: function (P, m, a, soft, thr) {
      var cx = 0, cz = 0, k;
      for (k = 0; k < m; k++) { cx += P[k * 3] + thr.x * P[k * 3 + 1]; cz += P[k * 3 + 2] + thr.z * P[k * 3 + 1]; }
      cx /= m; cz /= m;
      for (k = 0; k < m; k++) {
        var j = (k + 1) % m, ax = P[k * 3] + thr.x * P[k * 3 + 1] - cx, az = P[k * 3 + 2] + thr.z * P[k * 3 + 1] - cz, bx = P[j * 3] + thr.x * P[j * 3 + 1] - cx, bz = P[j * 3 + 2] + thr.z * P[j * 3 + 1] - cz;
        var qi = 1 - soft, ro = 1 + soft;
        vert(cx, cz, a); vert(cx + ax * qi, cz + az * qi, a); vert(cx + bx * qi, cz + bz * qi, a);
        vert(cx + ax * qi, cz + az * qi, a); vert(cx + ax * ro, cz + az * ro, 0); vert(cx + bx * ro, cz + bz * ro, 0);
        vert(cx + ax * qi, cz + az * qi, a); vert(cx + bx * ro, cz + bz * ro, 0); vert(cx + bx * qi, cz + bz * qi, a);
      }
    },
    done: function () { geo.attributes.position.needsUpdate = true; geo.attributes.aAlpha.needsUpdate = true; } };
}

// ---------- The effect ----------
// Timed on e, seconds from the start. Dread to 0.95, the crack held to 1.1, the burst and the pieces
// finding their places to about 1.95, time frozen to 2.9, the snap back to 3.28, the slam and its
// aftermath to 4.5.
function fxShatter(st, die, fi, th, o) {
  var T = st.T, hd = st.q.detail !== 'basic', s = die.size, ks = s / 1.25, g = die.group, zones = shatterZones(o), i, k;
  var c = g.position.clone(), qLand = g.quaternion.clone(), e = 0, own = [], pale = shatterPale(th), glass = die.see === 'glass', rnd = mulberry(6143);
  var T_CRACK = 0.95, T_BURST = 1.1, T_OUT = 0.85, T_SNAP = 2.9, T_SLAM = 3.28, DUR = 4.5;
  var sun0 = st.sun.intensity, sickCol = pale ? SHAT_SICK_DEEP : SHAT_SICK, redCol = pale ? SHAT_RED_DEEP : SHAT_RED;
  var v1 = new T.Vector3(), v2 = new T.Vector3(), qa = new T.Quaternion(), qb = new T.Quaternion(), qc = new T.Quaternion(), m4 = new T.Matrix4(), ZERO = new T.Matrix4().makeScale(0, 0, 0);
  var thr = shatterThrow(st);
  var burst = false, reformed = false, done = false, pieces = null, scarsAt = 0, pieceCast = [];

  // The page: a cold dim and the ring of stopped ticks, and the cracks that run out over it.
  var Rring = s * 3.3, Rd = Rring * 1.2 + 0.6 * s;
  var pageMat = shatterPageMat(st, ks, zones, c, Rd), pu = pageMat.userData.uni, page = new T.Mesh(new T.PlaneGeometry(Rd * 2, Rd * 2), pageMat);
  page.rotation.x = -Math.PI / 2; page.position.set(c.x, 0.004, c.z); page.renderOrder = -2; st.scene.add(page); own.push(page);
  var fis = shatterFissureLines(st, c, s, hd, zones, mulberry(2210)), fisMat = shatterFissureMat(st, SHAT_RED), fu = fisMat.userData.uni;
  var fisMesh = new T.Mesh(shatterRibbonGeo(T, fis.lines, 0.012), fisMat); fisMesh.frustumCulled = false; fisMesh.renderOrder = -1; fisMesh.visible = false; st.scene.add(fisMesh); own.push(fisMesh);

  // The cracks on the faces' light layers, and the 1.
  var gl = die.faces.map(function (fc, n) { return glowFace(die, n); }), net = shatterNet(die, fi, 8821), pk = die.faces.map(function () { return ''; }), ink = pale;
  function wipe(f) { if (ink) f.g.clearRect(0, 0, f.cv, f.cv); else f.clear(); }
  if (ink) { shatterInkMode(T, die.faces, true); gl.forEach(function (f) { wipe(f); f.commit(); }); }
  function paintFace(n, G, alpha, col, numCol) {
    var key = Math.round(G * 26) + '|' + Math.round(alpha * 20) + '|' + col + '|' + (n === fi ? numCol : '');
    if (key === pk[n]) return;
    pk[n] = key;
    var f = gl[n], w = 4.4 * f.lay.cv / 256; wipe(f);
    var polys = shatterCracksAt(net.faces[n], G);
    if (polys.length && alpha > 0.01) {
      f.g.globalAlpha = alpha;
      if (ink) shatterInkStrokes(f.g, polys, w, col === SHAT_CRACK ? SHAT_INK_SICK : col); else glowStrokes(f.g, polys, w, col);
      f.g.globalAlpha = 1;
    }
    if (n === fi && numCol) shatterNumeral(f, numCol, pale);
    f.commit();
  }
  function setK(n, k2) { shatterLevel(die.faces[n], k2, ink); }
  // The ink gives way to light, which is what the layers are left as.
  function toLight() {
    ink = false; shatterInkMode(T, die.faces, false);
    gl.forEach(function (f, n) { f.clear(); f.commit(); pk[n] = ''; if (n !== fi) setK(n, 0); });
  }

  // One light in HD, cold, from the key light's side; Basic leans on the glowing rims.
  var lt = null;
  if (hd) { lt = new T.PointLight(sickCol, 0, s * 16, 1.4); lt.position.set(c.x - 1.4 * s, 3.4 * s, c.z - 1.0 * s); st.scene.add(lt); own.push(lt); }
  var gap = zones.reduce(function (m, z) { return Math.min(m, Math.hypot(z.x - c.x, z.z - c.z) - z.r); }, 99);
  var flareSize = Math.max(s * 2.2, Math.min(s * 4.6, gap * 2)), flare = glow(st, starTex(T), SHAT_COLD, flareSize), halo = glow(st, spriteTex(T), sickCol, s * 3.0);
  flare.position.set(c.x, c.y + 0.3 * s, c.z); flare.renderOrder = 7;
  halo.position.copy(c); halo.renderOrder = 5;
  // Both are depth tested and sit inside the die, so they glow round it and never over its faces.
  st.scene.add(flare, halo); own.push(flare, halo);

  var glit = shatterPool(st, hd ? 190 : 96, { size: s * 0.24, tex: starTex(T), add: true, colors: [SHAT_COLD, '#ffffff', '#ffb0c0', '#bfe1ff'], boost: 1.15, g: s * 3, twinkle: 21, alpha: 1, order: 6 });
  var tink = shatterPool(st, hd ? 80 : 40, { size: s * 0.24, tex: starTex(T), add: true, colors: [SHAT_COLD, '#ffffff', '#ffb0c0'], boost: 1.3, g: s * 7, twinkle: 30, alpha: 1, order: 6 });
  var dust = shatterPool(st, hd ? 90 : 44, { size: s * 0.13, tex: spriteTex(T), add: false, colors: ['#8fa3bd', '#6a7d96', '#cfdceb'], g: s * 1.2, alpha: 0.75, order: 5 });
  own.push(glit.obj, tink.obj, dust.obj);

  // Chips: fine fragments of the die itself. They fly out, stop with everything else, and at the snap
  // all but a few fly back into the die; those few fall to the page and are lost.
  var NSH = Math.round((hd ? 70 : 36)), chipMat = new T.MeshPhysicalMaterial({ color: 0xffffff, roughness: glass ? 0.12 : 0.3, metalness: glass ? 0 : 0.1, clearcoat: 0.5, flatShading: true, side: T.DoubleSide,
    envMapIntensity: 1.3, emissive: new T.Color(SHAT_COLD), emissiveIntensity: 0.1, transparent: glass, opacity: glass ? 0.62 : 1 });
  var chips = new T.InstancedMesh(shatterChipGeo(T), chipMat, NSH), sh = [], gradC = th.base.grad, cc = new T.Color();
  chips.castShadow = !glass; chips.instanceColor && (chips.instanceColor.needsUpdate = true); chips.frustumCulled = false; chips.visible = false; chips.renderOrder = 4; st.scene.add(chips); own.push(chips);
  for (k = 0; k < NSH; k++) {
    var rr = mulberry(1200 + k), ang = rr() * 6.2832, el = (rr() - 0.15) * 1.3, sp = s * (4.5 + rr() * 7.5), D0 = 3.2;
    var p0 = new T.Vector3(c.x + Math.cos(ang) * s * 0.5 * rr(), c.y + (rr() - 0.5) * s * 0.9, c.z + Math.sin(ang) * s * 0.5 * rr());
    var vx = Math.cos(ang) * Math.cos(el) * sp, vz = Math.sin(ang) * Math.cos(el) * sp, vy = (Math.sin(el) * 0.6 + 0.55) * sp * 0.4, dest = { x: p0.x + vx / D0, z: p0.z + vz / D0 };
    shatterPush(st, dest, zones, s * 0.3, s * 0.5); vx = (dest.x - p0.x) * D0; vz = (dest.z - p0.z) * D0;
    var sz = s * (0.07 + 0.17 * Math.pow(rr(), 1.5)), hp = new T.Vector3(rr() - 0.5, rr() - 0.5, rr() - 0.5).normalize();
    sh.push({ p0: p0, v: new T.Vector3(vx, vy, vz), ax: new T.Vector3(rr() - 0.5, rr() - 0.5, rr() - 0.5).normalize(), w: (rr() - 0.5) * 26, qi: new T.Quaternion().setFromAxisAngle(hp, rr() * 6.2832),
      sc: new T.Vector3(sz, sz * (0.6 + rr() * 0.9), sz * (0.5 + rr() * 0.7)), d: rr() * 0.05, lost: k % 6 === 5, fd: rr() * 0.22, fell: false, tinks: 0, sc1: 0, vel: new T.Vector3(),
      p: new T.Vector3(), q: new T.Quaternion(), pS: new T.Vector3(), qS: new T.Quaternion(),
      home: new T.Vector3(c.x + (rr() - 0.5) * s * 0.8, c.y + (rr() - 0.5) * s * 0.8, c.z + (rr() - 0.5) * s * 0.8) });
    chips.setColorAt(k, cc.set(k % 4 === 0 ? gradC[0] : k % 4 === 1 ? gradC[1] : k % 4 === 2 ? mix(gradC[0], '#ffffff', 0.35) : mix(gradC[1], '#dfeaff', 0.2)));
    chips.setMatrixAt(k, ZERO);
  }

  // Soft shadows of the hanging pieces. Solid dice also cast real ones (below), so these are the
  // soft part; glass throws a tinted one.
  var corn = die.faces.map(function (fc) { return fc.f.idx.length; }), loc = die.faces.map(function (fc) { return fc.f.idx.map(function (q) { return die.sd.v[q]; }).map(function (v) { return { x: v.x * s - fc.c.x, y: v.y * s - fc.c.y, z: v.z * s - fc.c.z }; }); });
  var shd = shatterShadowMesh(st, corn, glass ? mix(die.th.tint || die.th.shadow || gradC[0], '#000000', 0.45) : '#0a0812'), shdA = glass ? 0.3 : hd ? 0.24 : 0.34, shP = new Float32Array(24);
  st.scene.add(shd.mesh); own.push(shd.mesh);

  var pale86 = pale ? 0.86 : 1, pullTo = { x: c.x, y: c.y, z: c.z, k: 16 };
  // How the pool particles move: full speed at the burst, then time stops.
  function kTime() { return e < T_BURST ? 1 : e < T_SNAP ? 0.04 + 0.96 * Math.exp(-(e - T_BURST) * 3.6) : 1; }

  // ----- the dread: the 1 flickers, cracks race, the crack -----
  function dread() {
    var pr = clamp01((e - 0.22) / 0.72), G = net.tmax * Math.pow(pr, 1.15), fl = shatterFlick(e, 0.18), hold = e >= T_CRACK;
    var nc = hold ? (pale ? '#8fbf00' : '#ffffff') : e < 0.7 ? sickCol : mix(sickCol, pale ? '#d4ff40' : '#ffffff', 0.4);
    var spike = hold ? Math.exp(-(e - T_CRACK) * 9) : 0;
    for (var n = 0; n < die.faces.length; n++) {
      var fn = net.faces[n];
      if (n !== fi && G < fn.tS) continue;
      paintFace(n, Math.min(G, fn.tE), 1, SHAT_CRACK, nc);
      setK(n, n === fi ? (1.9 + 0.7 * pr + 2.2 * spike) * fl : (1.0 + 1.3 * pr + 2.6 * spike) * (0.85 + 0.15 * fl));
    }
    tintDie(die, hold ? SHAT_COLD : sickCol, (0.1 + 0.5 * pr) * (0.6 + 0.4 * fl) + (hold ? 0.25 : 0));
    if (pale) shadeDie(die, 1 - 0.14 * smooth(0, 0.5, e));
    // The die shivers harder as it cracks, goes dead still for the crack, then hops as it gives.
    var j = s * 0.03 * smooth(0.4, 0.93, e) * (e < T_CRACK ? 1 : 0);
    var hop = hold ? s * 0.07 * Math.sin(Math.PI * (e - T_CRACK) / (T_BURST - T_CRACK)) : 0;
    g.position.set(c.x + (rnd() - 0.5) * j, c.y + (rnd() - 0.5) * j * 0.4 + hop, c.z + (rnd() - 0.5) * j);
    halo.material.opacity = (0.34 * smooth(0.05, 0.4, e) * (0.4 + 0.6 * fl) + (hold ? 0.5 : 0)) * (1 - smooth(T_CRACK + 0.1, T_BURST, e));
    halo.material.color.set(hold ? SHAT_COLD : sickCol);
    if (lt) { lt.color.set(hold ? SHAT_COLD : sickCol); lt.intensity = 8 * smooth(0.1, 0.7, e) * (0.5 + 0.5 * fl) + 32 * spike; }
    fisMesh.visible = hold;
  }

  // ----- the burst -----
  function doBurst() {
    burst = true; g.position.copy(c); g.quaternion.copy(qLand); g.updateMatrixWorld(true);
    var parts = splitDie(st, die);
    // The pieces are thin shells, which a shadow map draws unevenly; their shadows are drawn by hand.
    pieceCast = [];
    die._pieces.forEach(function (pp) { pp.meshes.forEach(function (x) { pieceCast.push([x, x.castShadow]); x.castShadow = false; }); });
    pieces = parts.map(function (p, n) {
      var r = mulberry(900 + n), nw = p.dir.clone(), hl = Math.hypot(nw.x, nw.z), az = (hl > 0.22 ? Math.atan2(nw.z, nw.x) : n * 2.39996) + (r() - 0.5) * 0.24;
      // The upper pieces rise highest and hang nearest the middle; the lower ones are swung out and
      // turn to face the viewer, so every piece, and its number, is seen from above.
      var rh = s * (0.5 + 1.9 * hl + (nw.y < -0.3 ? 0.75 : 0) + r() * 0.25), yh = s * (1.0 + 1.45 * (0.5 + 0.5 * nw.y) + r() * 0.2);
      var tp = shatterPush(st, { x: c.x + Math.cos(az) * rh, z: c.z + Math.sin(az) * rh }, zones, s * 0.55, s * 1.0), az2 = Math.atan2(tp.z - c.z, tp.x - c.x);
      var tilt = 0.16 + 0.5 * Math.min(1, Math.hypot(tp.x - c.x, tp.z - c.z) / (s * 2.4)), nT = new T.Vector3(Math.cos(az2) * Math.sin(tilt), Math.cos(tilt), Math.sin(az2) * Math.sin(tilt));
      var q0 = p.m.quaternion.clone(), qh = new T.Quaternion().setFromUnitVectors(nw, nT).multiply(q0);
      qh.premultiply(qa.setFromAxisAngle(nT, n === fi ? 0 : (r() - 0.5) * 6));
      var wa = r() * 6.2832;
      return { m: p.m, fc: p.fc, from: p.m.position.clone(), home: p.home.clone(), hang: new T.Vector3(tp.x, yh, tp.z), q0: q0, qh: qh, nT: nT, ax: new T.Vector3(r() - 0.5, r() - 0.5, r() - 0.5).normalize(),
        tum: (2.2 + 4 * r()) * (r() < 0.5 ? -1 : 1), w: (0.16 + 0.26 * r()) * (r() < 0.5 ? -1 : 1), wax: new T.Vector3(Math.cos(wa), 0, Math.sin(wa)), ph: r() * 6.2832, dx: Math.cos(az2), dz: Math.sin(az2),
        sw: r() < 0.5 ? -1 : 1, pS: new T.Vector3(), qS: new T.Quaternion(), snapped: false };
    });
    for (var n = 0; n < die.faces.length; n++) if (n !== fi) setK(n, 0);
    pk[fi] = ''; paintFace(fi, 0, 0, SHAT_CRACK, redCol);
    chips.visible = true; shd.mesh.visible = true;
    glit.emit(Math.round((hd ? 150 : 74)), function (j, r) {
      var a = r() * 6.283, el = (r() - 0.2) * 1.4, sp = s * (3 + r() * 9), rr2 = s * 0.6 * r(), v = [Math.cos(a) * Math.cos(el) * sp, Math.sin(el) * sp * 0.7 + s * 1.2, Math.sin(a) * Math.cos(el) * sp];
      var dst = shatterPush(st, { x: c.x + v[0] * 0.4, z: c.z + v[2] * 0.4 }, zones, s * 0.5, 0.3); v[0] = (dst.x - c.x) / 0.4; v[2] = (dst.z - c.z) / 0.4;
      return { p: [c.x + Math.cos(a) * rr2, c.y + (r() - 0.5) * s, c.z + Math.sin(a) * rr2], v: v, life: 2.3 + r() * 0.8, delay: r() * 0.05 };
    });
    dust.emit(Math.round((hd ? 60 : 30)), function (j, r) {
      var a = r() * 6.283, sp = s * (1.5 + r() * 5), v = [Math.cos(a) * sp, s * (0.4 + r() * 2.2), Math.sin(a) * sp], dst = shatterPush(st, { x: c.x + v[0] * 0.4, z: c.z + v[2] * 0.4 }, zones, s * 0.5, 0.3);
      v[0] = (dst.x - c.x) / 0.4; v[2] = (dst.z - c.z) / 0.4;
      return { p: [c.x + Math.cos(a) * s * 0.5 * r(), c.y + (r() - 0.5) * s * 0.8, c.z + Math.sin(a) * s * 0.5 * r()], v: v, life: 2.4 + r() * 0.9 };
    });
  }
  function placePiece(pc, n) {
    var m = pc.m, tt = e - T_BURST;
    if (e < T_SNAP) {
      var kk = out3(tt / T_OUT), an = smooth(T_SNAP - 0.25, T_SNAP, e);
      v1.copy(pc.from).lerp(pc.hang, kk);
      v1.y += Math.sin(kk * Math.PI) * s * 0.3 + Math.sin(e * 1.1 + pc.ph) * 0.035 * s * kk;
      // A breath out just before the snap.
      v1.x += pc.dx * an * s * 0.14; v1.z += pc.dz * an * s * 0.14; v1.y += an * s * 0.06;
      m.position.copy(v1);
      qa.copy(pc.q0).slerp(pc.qh, kk);
      qb.setFromAxisAngle(pc.ax, (1 - kk) * pc.tum);
      qc.setFromAxisAngle(pc.nT, n === fi ? 0.22 * Math.sin(0.9 * tt) : pc.w * tt);
      m.quaternion.copy(qc).multiply(qb).multiply(qa);
      qb.setFromAxisAngle(pc.wax, 0.06 * kk * Math.sin(1.2 * e + pc.ph)); m.quaternion.premultiply(qb);
    } else {
      if (!pc.snapped) { pc.snapped = true; pc.pS.copy(m.position); pc.qS.copy(m.quaternion); }
      var mm = clamp01((e - T_SNAP) / (T_SLAM - T_SNAP)), ease = Math.pow(mm, 2.2), sw = (1 - ease) * 0.5 * pc.sw;
      v1.copy(pc.pS).lerp(pc.home, ease);
      v2.set(v1.x - c.x, 0, v1.z - c.z);
      m.position.set(c.x + v2.x * Math.cos(sw) - v2.z * Math.sin(sw), v1.y, c.z + v2.x * Math.sin(sw) + v2.z * Math.cos(sw));
      qa.copy(pc.qS).slerp(pc.q0, ease); qb.setFromAxisAngle(pc.ax, -(1 - ease) * (1 - ease) * 1.6);
      m.quaternion.copy(qb).multiply(qa);
    }
    m.position.y = Math.max(0.05, m.position.y);
  }
  function updPieces() {
    var fade = smooth(T_BURST, T_BURST + 0.3, e) * (1 - smooth(T_SLAM - 0.18, T_SLAM, e));
    shd.begin();
    for (var n = 0; n < pieces.length; n++) {
      var pc = pieces[n], L = loc[n], m = L.length;
      placePiece(pc, n);
      // The shadow: the tile's outline thrown along the key light, wider, fainter and softer the higher it hangs.
      pc.m.updateMatrix();
      var h = pc.m.position.y, mx = 0, mz = 0;
      for (var q = 0; q < m; q++) { v1.set(L[q].x, L[q].y, L[q].z).applyMatrix4(pc.m.matrix); shP[q * 3] = v1.x; shP[q * 3 + 1] = v1.y; shP[q * 3 + 2] = v1.z; mx += v1.x + thr.x * v1.y; mz += v1.z + thr.z * v1.y; }
      shd.add(shP, m, shdA * fade * (1 - 0.4 * Math.min(1, h / (4 * s))) * (shatterIn(mx / m, mz / m, zones, 0) ? 0 : 1), 0.4 + 0.05 * h / s, thr);
    }
    shd.done();
    // The 1's piece burns cold red the whole time.
    setK(fi, 1.9 + 0.35 * Math.sin(e * 5) + 2.4 * Math.exp(-(e - T_BURST) * 5) + (e > T_SNAP ? 1.2 * smooth(T_SNAP, T_SLAM, e) : 0));
    var pf = pieces[fi].m.position;
    halo.position.set(pf.x, pf.y - 0.25 * s, pf.z); halo.material.color.set(redCol);
    halo.material.opacity = (0.11 + 0.03 * Math.sin(e * 5)) * smooth(T_BURST + 0.1, T_BURST + 0.4, e) * (1 - smooth(T_SLAM - 0.15, T_SLAM, e));
    // Rims glint cold while they hang, and flush red as they come in.
    tintDie(die, e < T_SNAP ? SHAT_COLD : redCol, e < T_SNAP ? 0.45 * smooth(T_BURST, T_BURST + 0.25, e) * (0.85 + 0.15 * Math.sin(e * 2.3)) : 0.5 + 0.5 * smooth(T_SNAP, T_SLAM, e));
  }
  // Scars are painted on the hidden layers while the pieces hang, a few faces a frame.
  function prepaintScars(all) {
    var cnt = 0;
    while (scarsAt < die.faces.length && (all || cnt < 3)) { if (scarsAt !== fi) { paintFace(scarsAt, net.tmax + 1, 1, redCol, ''); cnt++; } scarsAt++; }
  }
  function updChips(dt) {
    var tt = e - T_BURST, tmp = v2;
    for (var n = 0; n < NSH; n++) {
      var S = sh[n], tau = Math.max(0, tt - S.d), a1 = (1 - Math.exp(-3.2 * tau)) / 3.2, f1 = a1 + 0.03 * tau;
      if (e < T_SNAP) {
        S.p.set(S.p0.x + S.v.x * f1, Math.max(0.1, S.p0.y + S.v.y * f1), S.p0.z + S.v.z * f1);
        qa.setFromAxisAngle(S.ax, S.w * (a1 + 0.1 * tau)); S.q.copy(qa).multiply(S.qi);
        S.sc1 = smooth(0, 0.05, tau); S.pS.copy(S.p); S.qS.copy(S.q);
      } else if (!S.lost) {
        var mm = clamp01((e - T_SNAP) / (T_SLAM - T_SNAP)), ease = Math.pow(mm, 2.2);
        S.p.copy(S.pS).lerp(S.home, ease); S.q.copy(S.qS).slerp(S.qi, ease); S.sc1 = 1 - smooth(0.7, 1, mm);
      } else {
        var lt2 = e - T_SNAP - S.fd;
        if (lt2 > 0) {
          if (!S.fell) { S.fell = true; S.p.copy(S.pS); S.q.copy(S.qS); S.vel.set((rnd() - 0.5) * s * 0.8, -s * 0.4, (rnd() - 0.5) * s * 0.8); }
          S.vel.y -= s * 22 * dt; S.p.addScaledVector(S.vel, dt);
          var rest = S.sc.y * 0.4 + 0.02;
          if (S.p.y < rest) {
            S.p.y = rest;
            if (S.vel.y < -s * 1.4 && S.tinks < 3) {
              S.tinks++;
              var bx = S.p.x, bz = S.p.z;
              tink.emit(hd ? 5 : 3, function (j, r) { return { p: [bx, 0.08, bz], v: [(r() - 0.5) * s * 2, s * (1.5 + r() * 2.5), (r() - 0.5) * s * 2], life: 0.45 + r() * 0.5 }; });
            }
            S.vel.y *= -0.35; S.vel.x *= 0.6; S.vel.z *= 0.6;
            if (Math.abs(S.vel.y) < s * 0.9) S.vel.y = 0;
          }
          S.q.premultiply(qa.setFromAxisAngle(S.ax, S.w * dt * (S.p.y > rest + 0.01 ? 1 : 0.1)));
        } else { S.p.copy(S.pS); S.q.copy(S.qS); }
        S.sc1 = 1 - smooth(DUR - 1.1, DUR - 0.35, e);
      }
      tmp.set(S.sc.x * S.sc1, S.sc.y * S.sc1, S.sc.z * S.sc1);
      m4.compose(S.p, S.q, tmp); chips.setMatrixAt(n, m4);
    }
    chips.instanceMatrix.needsUpdate = true;
  }

  // ----- the slam -----
  function reform() {
    reformed = true; g.position.copy(c); g.quaternion.copy(qLand); wholeDie(die);
    pieceCast.forEach(function (x) { x[0].castShadow = x[1]; }); shd.mesh.visible = false;
    prepaintScars(true);
    tink.emit(Math.round((hd ? 40 : 20)), function (j, r) { var a = r() * 6.283, sp = s * (2 + r() * 5); return { p: [c.x + Math.cos(a) * s * 0.7, 0.1, c.z + Math.sin(a) * s * 0.7], v: [Math.cos(a) * sp, s * (1 + r() * 3.5), Math.sin(a) * sp], life: 0.5 + r() * 0.7 }; });
    dust.emit(Math.round((hd ? 24 : 12)), function (j, r) { var a = r() * 6.283, sp = s * (1 + r() * 3); return { p: [c.x + Math.cos(a) * s * 0.7, 0.1, c.z + Math.sin(a) * s * 0.7], v: [Math.cos(a) * sp, s * (0.3 + r() * 1.2), Math.sin(a) * sp], life: 0.8 + r() * 0.8 }; });
  }
  function aftermath() {
    var ta = e - T_SLAM, f = 1 - smooth(0.3, 1.25, ta), sq = 1 - 0.07 * Math.exp(-ta * 9) * Math.cos(ta * 30);
    g.scale.setScalar(sq);
    if (ink && ta > 0.95) toLight();
    if (ink || !pale) for (var n = 0; n < die.faces.length; n++) if (n !== fi) setK(n, 1.5 * f);
    paintFace(fi, net.tmax + 1, f, redCol, redCol);
    setK(fi, (pale ? 2.0 : 1.75) + 1.2 * Math.exp(-ta * 4.5));
    tintDie(die, redCol, 0.55 * f * (1 + 0.5 * Math.exp(-ta * 6)));
    halo.material.color.set(redCol); halo.material.opacity = 0.3 * Math.exp(-ta * 3.5);
    halo.position.copy(c);
  }

  return { update: function (dt) {
    if (done) return false;
    e += dt;
    // Page: the cold dim and the ring. The ring opens at the burst, hangs, then closes as time snaps.
    var sn = clamp01((e - T_SNAP) / (T_SLAM - T_SNAP));
    pu.uTime.value = e;
    pu.uDim.value = 0.38 * smooth(0.1, 0.9, e) * (1 - smooth(3.55, 4.3, e));
    pu.uR.value = e < T_BURST ? 0 : Rring * (e < T_SNAP ? out3((e - T_BURST) / 0.6) : 1 - in3(sn));
    pu.uA.value = (e < T_BURST ? 0 : 0.9) * (1 - smooth(T_SLAM - 0.04, T_SLAM + 0.06, e));
    pu.uRot.value = (e < T_SNAP ? 0.07 * (e - T_BURST) : 0.07 * (T_SNAP - T_BURST) - 7 * sn * sn);
    pu.uFlash.value = e >= T_BURST ? 0.8 * Math.exp(-(e - T_BURST) * 5) + (e >= T_SNAP ? 0.9 * sn : 0) : 0;
    // The shock rings: a small one at the burst, a larger one at the slam.
    if (e >= T_SLAM) { var sk = clamp01((e - T_SLAM) / 0.7); pu.uShockR.value = 0.3 + s * 4.6 * out3(sk); pu.uShockA.value = Math.pow(1 - sk, 1.5); }
    else { var bk = clamp01((e - T_BURST) / 0.5); pu.uShockR.value = e >= T_BURST ? 0.3 + s * 3.6 * out3(bk) : 0; pu.uShockA.value = 0.75 * Math.pow(1 - bk, 1.5); }
    // The cracks on the page run out at the crack, glow, and are drawn back in at the snap.
    fu.uReach.value = fis.reach * out3((e - T_CRACK) / 0.3) * (1 - in3(sn));
    fu.uHot.value = 1.3 * Math.exp(-(e - T_CRACK) * 4) + 0.8 * sn;
    fu.uFade.value = 0.5 + 0.5 * Math.exp(-(e - T_CRACK) * 1.2);
    st.sun.intensity = sun0 * (1 - 0.3 * smooth(0.3, 1.0, e) * (1 - smooth(3.5, 4.2, e)));
    if (e < T_BURST) dread();
    if (e >= T_BURST && !burst) doBurst();
    if (burst && !reformed) {
      updPieces(); updChips(dt);
      if (e > T_BURST + 0.35 && e < T_SNAP) prepaintScars(false);
    }
    if (e >= T_SLAM && !reformed) reform();
    if (reformed) { updChips(dt); aftermath(); }
    // Light and flare.
    var sl = e - T_SLAM, fl2 = 0.3 * (e >= T_CRACK ? Math.exp(-(e - T_CRACK) * 12) : 0) + (e >= T_BURST ? 0.5 * Math.exp(-(e - T_BURST) * 6) : 0) + (e >= T_SNAP && e < T_SLAM ? 0.18 * sn * sn : 0) + (e >= T_SLAM ? 0.4 * Math.exp(-sl * 7) : 0);
    flare.material.opacity = Math.min(1, fl2); flare.scale.setScalar(flareSize * (0.7 + 0.5 * Math.min(1, fl2))); flare.material.rotation = e * 0.3;
    flare.material.color.set(e >= T_SLAM ? '#ffd0d8' : SHAT_COLD);
    if (lt && e >= T_BURST) {
      lt.color.set(e >= T_SLAM ? redCol : SHAT_COLD);
      lt.intensity = 60 * Math.exp(-(e - T_BURST) * 7) + 9 * smooth(T_BURST + 0.2, T_BURST + 0.6, e) * (1 - smooth(T_SNAP, T_SLAM, e)) + 30 * sn + (e >= T_SLAM ? 90 * Math.exp(-sl * 8) + 5 * Math.exp(-sl * 1.3) : 0);
    }
    var kT = kTime(), pull = e >= T_SNAP + 0.04 && e < T_SLAM ? pullTo : null;
    glit.update(dt, kT, 1 - smooth(T_SNAP + 0.04, T_SLAM, e), pull);
    tink.update(dt, 1, 1 - smooth(DUR - 0.5, DUR - 0.05, e), null);
    dust.update(dt, kT, 1 - smooth(DUR - 0.7, DUR - 0.1, e), null);
    if (e >= DUR) {
      done = true; g.position.copy(c); g.quaternion.copy(qLand); g.scale.setScalar(1); st.sun.intensity = sun0;
      tintDie(die, th.burn, 0); shadeDie(die, pale86);
      if (ink) toLight();
      for (var n = 0; n < die.faces.length; n++) if (n !== fi) setK(n, 0);
      // The 1 stays lit cold red.
      pk[fi] = ''; paintFace(fi, 0, 0, SHAT_CRACK, redCol); setK(fi, pale ? 2.0 : 1.75);
      chips.dispose();
      drop(st, own);
      return false;
    }
    return true;
  } };
}

// ---------- The echo: a crack runs across one face and seals ----------
// The same thing, quieter: a hairline crack (first along the page from the grand die, when there is one)
// races across the echo die's top face with a puff of dust, glows, then seals shut from the end it
// started at, leaving the 1 lit cold red. Nothing leaves the die.
function fxShatterEcho(st, die, fi, th, ev) {
  var T = st.T, hd = st.q.detail !== 'basic', s = die.size, ks = s / 1.25, fc = die.faces[fi], gf = glowFace(die, fi), pale = shatterPale(th), lay = fc.lay, cv = lay.cv, rnd = mulberry(4407 + fi), ink = pale;
  var c = die.group.position.clone(), own = [], t = 0, redCol = pale ? SHAT_RED_DEEP : SHAT_RED, sickCol = pale ? SHAT_SICK_DEEP : SHAT_SICK, from = ev && ev.from ? ev.from.group.position : null;
  var finalK = pale ? 1.7 : 1.4, DUR;

  // The crack across the top face: from a point on one rim, through near the middle, out the far side.
  var poly = lay.poly, np = poly.length, e1 = Math.floor(rnd() * np), u1 = 0.3 + rnd() * 0.4, A = [poly[e1][0] + (poly[(e1 + 1) % np][0] - poly[e1][0]) * u1, poly[e1][1] + (poly[(e1 + 1) % np][1] - poly[e1][1]) * u1], B;
  if (np === 3) { var vtx = poly[(e1 + 2) % 3]; B = [vtx[0] + (cv / 2 - vtx[0]) * 0.1, vtx[1] + (cv / 2 - vtx[1]) * 0.1]; }
  else { var e2 = (e1 + Math.floor(np / 2)) % np, u2 = 0.3 + rnd() * 0.4; B = [poly[e2][0] + (poly[(e2 + 1) % np][0] - poly[e2][0]) * u2, poly[e2][1] + (poly[(e2 + 1) % np][1] - poly[e2][1]) * u2]; }
  var Wp = [cv / 2 + (rnd() - 0.5) * cv * 0.16, cv / 2 + (rnd() - 0.5) * cv * 0.16], pl = shatterJag(A, Wp, rnd, cv * 0.03, cv * 0.055).concat(shatterJag(Wp, B, rnd, cv * 0.03, cv * 0.055).slice(1));
  var jb = Math.floor(pl.length * (0.4 + rnd() * 0.2)), paths = [pl, shatterBranch(pl, jb, rnd, cv * 0.12, cv * 0.02)];

  // From the grand die along the page to this one.
  var tPage = 0, fis = null, fu = null, fisMesh = null, plen = 0;
  if (from) {
    var dx = c.x - from.x, dz = c.z - from.z, L = Math.hypot(dx, dz);
    if (L > s * 1.6) {
      var ux = dx / L, uz = dz / L, n = Math.max(4, Math.ceil(L / (s * 0.45))), pts = [], endL = L - s * 0.9;
      for (var i = 0; i <= n; i++) {
        var u = i / n, off = (i === 0 || i === n) ? 0 : (rnd() - 0.5) * s * 0.5 * Math.sin(u * Math.PI);
        pts.push([from.x + ux * endL * u - uz * off, from.z + uz * endL * u + ux * off]);
      }
      plen = endL; tPage = Math.max(0.35, Math.min(0.6, 0.28 + L * 0.03));
      fisMesh = new T.Mesh(shatterRibbonGeo(T, [{ pts: pts, d0: 0, w: 0.04 * ks }], 0.014), shatterFissureMat(st, redCol));
      fisMesh.frustumCulled = false; fisMesh.renderOrder = 60; st.scene.add(fisMesh); own.push(fisMesh); fu = fisMesh.material.userData.uni;
    }
  }
  var t0 = tPage * 0.6, T_RACE = 0.26, T_GLOW = 0.7, T_SEAL = 1.2; DUR = t0 + 1.55;

  var Rd = s * 2.6, dimMat = shatterPageMat(st, ks, [], c, Rd), du = dimMat.userData.uni, dim = new T.Mesh(new T.PlaneGeometry(Rd * 2, Rd * 2), dimMat);
  dim.rotation.x = -Math.PI / 2; dim.position.set(c.x, 0.004, c.z); dim.renderOrder = -2; st.scene.add(dim); own.push(dim);
  var dust = shatterPool(st, hd ? 18 : 9, { size: s * 0.13, tex: spriteTex(T), add: false, colors: ['#8fa3bd', '#6a7d96', '#cfdceb'], g: -s * 0.35, alpha: 0.75, order: 61 });
  var glit = shatterPool(st, hd ? 16 : 8, { size: s * 0.22, tex: starTex(T), add: true, colors: [SHAT_COLD, '#ffffff', '#ffb0c0'], boost: 1.2, g: s * 0.3, twinkle: 26, alpha: 1, order: 62 });
  var spark = glow(st, starTex(T), SHAT_COLD, s * 0.9); spark.renderOrder = 63; spark.material.depthTest = false; st.scene.add(spark); own.push(dust.obj, glit.obj, spark);
  var keyDraw = '', puffed = false, ringed = false, crk2 = pale ? redCol : SHAT_CRACK;
  function lvl(k) { shatterLevel(fc, k, ink); }
  function toLight() { ink = false; shatterInkMode(T, [fc], false); gf.clear(); gf.commit(); keyDraw = ''; }
  if (ink) { shatterInkMode(T, [fc], true); gf.g.clearRect(0, 0, cv, cv); gf.commit(); }
  function worldAt(p) { var l = faceToLocal(die, fi, p[0], p[1]); return worldOf(die, l); }
  function headAt(p) { var pts = cutPaths([pl], p)[0]; return pts[pts.length - 1]; }
  function draw(a, b, col, alpha, numCol) {
    var key = Math.round(a * 28) + '|' + Math.round(b * 28) + '|' + col + '|' + Math.round(alpha * 20) + '|' + numCol;
    if (key === keyDraw) return; keyDraw = key;
    if (ink) gf.g.clearRect(0, 0, cv, cv); else gf.clear();
    if (b > a + 0.001 && alpha > 0.01) {
      var segs = paths.map(function (q, qi) { return shatterSlice(q, qi ? Math.min(a, 0.9) : a, qi ? Math.min(b, 1) : b); }).filter(function (q) { return q.length > 1; });
      gf.g.globalAlpha = alpha;
      if (ink) shatterInkStrokes(gf.g, segs, 3.4 * cv / 256, col === SHAT_CRACK ? SHAT_INK_SICK : col); else glowStrokes(gf.g, segs, 3.4 * cv / 256, col);
      gf.g.globalAlpha = 1;
    }
    if (numCol) shatterNumeral(gf, numCol, pale);
    gf.commit();
  }

  return { update: function (dt) {
    t += dt;
    var tt = t - t0;
    // The page crack runs from the grand die to this one, glows, and fades.
    if (fisMesh) {
      fu.uReach.value = plen * clamp01(t / tPage); fu.uHot.value = 1.1 * (1 - smooth(tPage, tPage + 0.5, t)); fu.uFade.value = 0.9 * (1 - smooth(tPage + 0.2, tPage + 0.9, t));
      if (t > tPage + 1) fisMesh.visible = false;
    }
    du.uDim.value = 0.27 * smooth(t0 - 0.1, t0 + 0.3, t) * (1 - smooth(DUR - 0.7, DUR - 0.1, t));
    var fl = shatterFlick(t, 0.25), hot = '#ffffff';
    if (tt < 0) {
      // Waiting for the crack to arrive: the 1 stirs.
      if (t > t0 - 0.2) { draw(0, 0, SHAT_CRACK, 0, sickCol); lvl(1.4 * fl * smooth(t0 - 0.2, t0, t)); }
    } else if (tt < T_RACE) {
      var p = tt / T_RACE;
      draw(0, p, SHAT_CRACK, 1, tt < 0.12 ? sickCol : mix(sickCol, pale ? '#d4ff40' : '#ffffff', 0.4));
      lvl((2.0 + 1.2 * p) * (0.7 + 0.3 * fl));
      var hp = worldAt(headAt(Math.max(0.02, p))); spark.position.set(hp.x, hp.y + 0.05, hp.z); spark.material.opacity = 0.6 * (1 - p * 0.5); spark.scale.setScalar(s * (0.45 + 0.25 * p));
      if (!puffed && p > 0.35) {
        puffed = true; var mp = worldAt(pl[Math.floor(pl.length / 2)]);
        dust.emit(hd ? 14 : 7, function (j, r) { return { p: [mp.x + (r() - 0.5) * s * 0.3, mp.y + 0.05, mp.z + (r() - 0.5) * s * 0.3], v: [(r() - 0.5) * s * 1.0, s * (0.5 + r() * 0.9), (r() - 0.5) * s * 1.0], life: 0.8 + r() * 0.6 }; });
        glit.emit(hd ? 12 : 6, function (j, r) { return { p: [mp.x, mp.y + 0.05, mp.z], v: [(r() - 0.5) * s * 2.2, s * (1 + r() * 2), (r() - 0.5) * s * 2.2], life: 0.5 + r() * 0.5 }; });
      }
    } else if (tt < T_GLOW) {
      var gk = (tt - T_RACE) / (T_GLOW - T_RACE);
      draw(0, 1, crk2, 1, redCol);
      lvl((2.4 + 0.6 * Math.sin(gk * 14)) * (1 - 0.1 * gk)); spark.material.opacity *= 0.7;
      die.mat.emissive.set(redCol); die.mat.emissiveIntensity = 0.3 * Math.sin(Math.PI * gk);
    } else if (tt < T_SEAL) {
      // Seals shut from the end it began at, a bright seam running along it.
      var sk = (tt - T_GLOW) / (T_SEAL - T_GLOW), a = out3(sk) * 0.98;
      draw(a, 1, crk2, 1, redCol);
      lvl(2.2 - 0.5 * sk);
      var sp2 = worldAt(headAt(Math.max(0.02, a))); spark.position.set(sp2.x, sp2.y + 0.05, sp2.z); spark.material.opacity = 0.5 * (1 - sk * 0.6); spark.scale.setScalar(s * 0.45);
      die.mat.emissiveIntensity = 0.3 * (1 - sk);
    } else {
      var rk = (tt - T_SEAL) / 0.35;
      if (ink && rk > 0.3) toLight();
      draw(1, 1, SHAT_CRACK, 0, redCol);
      lvl(finalK + 1.0 * (1 - clamp01(rk))); spark.material.opacity = 0; die.mat.emissiveIntensity = 0;
      ringed = true;
    }
    if (ringed) { var wk = clamp01((tt - T_SEAL) / 0.55); du.uShockR.value = 0.3 + s * 1.9 * out3(wk); du.uShockA.value = 0.55 * Math.pow(1 - wk, 1.5); }
    dust.update(dt, 1, 1 - smooth(DUR - 0.5, DUR - 0.05, t), null); glit.update(dt, 1, 1 - smooth(DUR - 0.5, DUR - 0.05, t), null);
    if (pale) shadeDie(die, 1 - 0.12 * smooth(t0 - 0.2, t0 + 0.2, t));
    if (t >= DUR) {
      die.mat.emissiveIntensity = 0; die.mat.emissive.set(th.burn);
      if (ink) toLight();
      draw(1, 1, SHAT_CRACK, 0, redCol); lvl(finalK);
      drop(st, own);
      return false;
    }
    return true;
  } };
}
registerCrit('shatter', { name: 'Shatter', kind: 'fail', what: 'The die cracks like glass, the pieces hang in the air, then snap back together.', fx: fxShatter, beat: 1.1, peak: 3.0,
  echo: fxShatterEcho, echoWhat: 'A crack runs across one face and seals.' });
