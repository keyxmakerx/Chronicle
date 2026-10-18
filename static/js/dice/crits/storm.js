// ---------- Crit success: Storm call ----------
// Storm cloud rolls in over the page from every side and turns round the die, which sits in a clear
// eye. The die charges, arcs crawling over its rims, then forked lightning lashes into it from the
// cloud wall, one bolt after another, each lighting the die and the page and burning a branching
// scorch round it. The last bolt drops from straight overhead: a blinding flash and a shockwave that
// throws the cloud back. The arcs settle on the 20, the scorch cools and the sky clears.
import {
  canvas, clamp01, dimmer, drop, faceToLocal, glow, glowFace, glowNumeral, mix, mulberry,
  numberOnto, out3, registerCrit, shadeDie, smooth, spray, spriteTex, starTex, tintDie, worldOf
} from '../engine.js';

var STORM_WHITE = '#dff4ff', STORM_BLUE = '#7cc8ff', STORM_VIOLET = '#9a7bff', STORM_NUM = '#b8e4ff';

// A seeded random with no allocation. Bolts are rebuilt every frame from their seeds, so one seed must
// always give the same channel, and a new flicker seed gives new fine detail on that same channel.
var stormS = 1;
function stormSeed(n) { stormS = (Math.imul(n | 0, 2654435761) ^ 0x5bd1e995) | 0; }
function stormRnd() { var t = stormS = (stormS + 0x6D2B79F5) | 0; t = Math.imul(t ^ t >>> 15, 1 | t); t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296; }

// A jagged channel from a to b by midpoint displacement, written as xyz into P from point o: 2^D + 1
// points. Each midpoint moves sideways by a share of its own segment's length, so the line is jagged
// at every scale, as lightning is. The first `keep` levels come from seedK and hold the channel's
// shape; finer levels come from seedF, which changes on every flicker.
function stormChannel(P, o, ax, ay, az, bx, by, bz, D, rough, seedK, seedF, keep) {
  var n = (1 << D) + 1, dx = bx - ax, dy = by - ay, dz = bz - az, L = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
  dx /= L; dy /= L; dz /= L;
  // Sideways basis: e1 lies flat on the page (from above, that is the zigzag you see), e2 the other way.
  var e1x = -dz, e1z = dx, h = Math.sqrt(e1x * e1x + e1z * e1z);
  if (h < 1e-3) { e1x = 1; e1z = 0; h = 1; }
  e1x /= h; e1z /= h;
  var e2x = dy * e1z, e2y = dz * e1x - dx * e1z, e2z = -dy * e1x;
  var i = o * 3, j = (o + n - 1) * 3;
  P[i] = ax; P[i + 1] = ay; P[i + 2] = az; P[j] = bx; P[j + 1] = by; P[j + 2] = bz;
  stormSeed(seedK);
  for (var step = n - 1, lv = 0; step > 1; step >>= 1, lv++) {
    if (lv === keep) stormSeed(seedF);
    var hs = step >> 1;
    for (var k = 0; k < n - 1; k += step) {
      var a = (o + k) * 3, b = (o + k + step) * 3, m = (o + k + hs) * 3;
      var sx = P[b] - P[a], sy = P[b + 1] - P[a + 1], sz = P[b + 2] - P[a + 2], sl = Math.sqrt(sx * sx + sy * sy + sz * sz);
      var u = (stormRnd() - 0.5) * 2 * rough * sl, w = (stormRnd() - 0.5) * 1.1 * rough * sl;
      P[m] = (P[a] + P[b]) * 0.5 + e1x * u + e2x * w;
      P[m + 1] = (P[a + 1] + P[b + 1]) * 0.5 + e2y * w;
      P[m + 2] = (P[a + 2] + P[b + 2]) * 0.5 + e1z * u + e2z * w;
    }
  }
  return n;
}

// All the lightning in one mesh: every channel is a ribbon turned to face the camera, rebuilt each
// frame. One draw gives a white-hot core that adds light, inside a blue-to-violet halo that also
// partly covers the page, so a bolt still reads on a white page, where added light alone would vanish.
var STORM_RIB_VS = [
  'attribute vec2 aV; attribute vec3 aI; varying vec2 vV; varying vec3 vI;',
  'void main(){ vV = aV; vI = aI; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }'].join('\n');
var STORM_RIB_FS = [
  'uniform vec3 uCore, uBlue, uViolet, uDeep; uniform float uDk; varying vec2 vV; varying vec3 vI;',
  'void main(){',
  ' float v = abs(vV.y), cf = vI.y, I = vI.x;',
  ' float core = 1.0 - smoothstep(cf * 0.4, cf, v);',
  ' float inner = exp(-v * v / (cf * cf * 7.0));',
  ' float halo = pow(1.0 - v, 2.6);',
  ' vec3 hc = mix(uBlue, uViolet, smoothstep(0.08, 0.7, v) * (0.55 + 0.45 * vI.z));',
  // uDk: a dark fringe that covers the page (alpha with a deep blue, no added light), for bolts that
  // cross a bare page rather than a dark cloud.
  ' float fr = (1.0 - v) * uDk, lite = 1.0 - 0.85 * uDk * smoothstep(0.12, 0.6, v);',
  ' vec3 col = (uCore * core * 2.6 + (uBlue * inner * 0.9 + hc * halo * 0.5) * lite + uDeep * fr * 0.5) * I;',
  ' gl_FragColor = vec4(col, clamp((halo * 0.42 + inner * 0.3 + fr * 0.85) * I, 0.0, 0.85));',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function stormRibbons(st, cap, order, dark) {
  var T = st.T, NV = cap * 2;
  var pos = new Float32Array(NV * 3), av = new Float32Array(NV * 2), ai = new Float32Array(NV * 3), ix = new Uint16Array(cap * 6);
  var geo = new T.BufferGeometry();
  var pa = new T.BufferAttribute(pos, 3), va = new T.BufferAttribute(av, 2), ia = new T.BufferAttribute(ai, 3), xa = new T.BufferAttribute(ix, 1);
  [pa, va, ia, xa].forEach(function (b) { b.setUsage(T.DynamicDrawUsage); });
  geo.setAttribute('position', pa); geo.setAttribute('aV', va); geo.setAttribute('aI', ia); geo.setIndex(xa); geo.setDrawRange(0, 0);
  // Premultiplied: colour adds, alpha only says how much of the page the halo covers.
  var mat = new T.ShaderMaterial({ vertexShader: STORM_RIB_VS, fragmentShader: STORM_RIB_FS, transparent: true, depthWrite: false, depthTest: false, side: T.DoubleSide, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    uniforms: { uCore: { value: new T.Color('#f4fbff') }, uBlue: { value: new T.Color(STORM_BLUE) }, uViolet: { value: new T.Color(STORM_VIOLET) }, uDeep: { value: new T.Color('#1a2a8a') }, uDk: { value: dark || 0 } } });
  var mesh = new T.Mesh(geo, mat); mesh.frustumCulled = false; mesh.renderOrder = order || 7; mesh.visible = false;
  var cam = st.cam.position, MAXL = 320, P = new Float32Array(MAXL * 3), W = new Float32Array(MAXL), I = new Float32Array(MAXL), nv = 0, ni = 0;
  return { mesh: mesh, P: P, W: W, I: I,
    begin: function () { nv = 0; ni = 0; },
    // Adds the polyline in P/W/I (n points): core is the white core's share of the width, tint 0..1
    // how far the halo leans to violet.
    push: function (n, core, tint) {
      if (n < 2 || nv + n * 2 > NV) return;
      var base = nv, lx = 0, ly = 0, lz = 0;
      for (var i = 0; i < n; i++) {
        var a = (i > 0 ? i - 1 : 0) * 3, b = (i < n - 1 ? i + 1 : n - 1) * 3, p = i * 3;
        var tx = P[b] - P[a], ty = P[b + 1] - P[a + 1], tz = P[b + 2] - P[a + 2];
        var vx = cam.x - P[p], vy = cam.y - P[p + 1], vz = cam.z - P[p + 2];
        var sx = ty * vz - tz * vy, sy = tz * vx - tx * vz, sz = tx * vy - ty * vx, sl = Math.sqrt(sx * sx + sy * sy + sz * sz);
        // A segment pointing at the camera has no side; keep the last one, and never let it flip.
        if (sl > 1e-7) { sx /= sl; sy /= sl; sz /= sl; if (i && sx * lx + sy * ly + sz * lz < 0) { sx = -sx; sy = -sy; sz = -sz; } lx = sx; ly = sy; lz = sz; }
        else if (i) { sx = lx; sy = ly; sz = lz; } else { sx = 1; sy = 0; sz = 0; lx = 1; }
        var w = W[i], q = nv * 3, u = i / (n - 1), r = nv * 2;
        pos[q] = P[p] - sx * w; pos[q + 1] = P[p + 1] - sy * w; pos[q + 2] = P[p + 2] - sz * w;
        pos[q + 3] = P[p] + sx * w; pos[q + 4] = P[p + 1] + sy * w; pos[q + 5] = P[p + 2] + sz * w;
        av[r] = u; av[r + 1] = -1; av[r + 2] = u; av[r + 3] = 1;
        ai[q] = I[i]; ai[q + 1] = core; ai[q + 2] = tint; ai[q + 3] = I[i]; ai[q + 4] = core; ai[q + 5] = tint;
        nv += 2;
      }
      for (i = 0; i < n - 1; i++) { var k = base + i * 2; ix[ni++] = k; ix[ni++] = k + 1; ix[ni++] = k + 2; ix[ni++] = k + 1; ix[ni++] = k + 3; ix[ni++] = k + 2; }
    },
    end: function () { geo.setDrawRange(0, ni); pa.needsUpdate = va.needsUpdate = ia.needsUpdate = xa.needsUpdate = true; mesh.visible = ni > 0; }
  };
}

// ---------- One bolt ----------
// A bolt is planned once (its path, branches and the pulses of its return strokes) and redrawn every
// frame from that plan, so the same bolt can be a side strike, the last one from overhead, or the
// small one that jumps to an echo die. open: visible from its first point (it does not start inside a
// cloud), k scales its width, gain its brightness.
function stormPlan(rnd, s, hd, t0, ax, ay, az, hit, final, seed) {
  var b = { t0: t0, lead: final ? 0.07 : 0.1, a: [ax, ay, az], b: [hit.x, hit.y + (final ? s * 0.02 : 0), hit.z], final: final, seed: seed, hd: hd,
    D: hd ? 6 : 5, keep: 3, rough: final ? 0.13 : 0.24, br: [], pulses: [], struck: false, open: false, k: 1, gain: 1 };
  var nb = final ? (hd ? 9 : 6) : (hd ? 6 : 4), i;
  for (i = 0; i < nb; i++) b.br.push({ u: final ? 0.06 + rnd() * 0.6 : 0.1 + rnd() * 0.62, side: rnd() < 0.5 ? -1 : 1,
    ang: final ? i / nb * 6.283 + (rnd() - 0.5) * 0.5 : 0.35 + rnd() * 0.5, droop: final ? 0.25 + rnd() * 0.35 : 0.1 + rnd() * 0.3,
    len: final ? s * (1.6 + rnd() * 1.5) : 0.22 + rnd() * 0.25, sub: rnd() < 0.6 });
  // A bright first stroke, then weaker re-strokes down the same channel.
  var tt = 0, lv = 1, np = final ? 5 : 4;
  for (i = 0; i < np; i++) { var on = (i ? 0.03 : 0.05) + rnd() * 0.02; b.pulses.push([tt, tt + on, lv]); tt += on + 0.025 + rnd() * 0.03; lv = (final ? 0.55 : 0.4) + rnd() * 0.4; }
  b.end = tt + 0.06;
  return b;
}

// The bolt as it stands tau seconds after its return stroke, pushed into the ribbons rb. Before the
// stroke (tau < 0) it is the leader: dim, feeling its way down in steps. Returns how bright the stroke
// is now (0 while it is only a leader), for the lights and flashes that follow it.
var STORM_M = new Float32Array(((1 << 6) + 1) * 3);
function stormBolt(rb, s, b, tau) {
  if (tau < -b.lead || tau > b.end) return 0;
  var M = STORM_M, hd = b.hd, gn = b.gain, lead = tau < 0, lv = 0.3, pk = 0, f = 1, i, k, last = b.pulses.length - 1, uu;
  if (lead) { pk = Math.floor((tau + b.lead) / b.lead * 7); f = (pk + 1) / 7; }
  else for (i = 0; i <= last; i++) {
    var pp = b.pulses[i]; if (tau < pp[0]) break;
    pk = 10 + i; lv = tau < pp[1] ? pp[2] * (1 - 0.35 * (tau - pp[0]) / (pp[1] - pp[0])) : 0.07 * (i < last ? 1 : 1 - clamp01((tau - pp[1]) / (b.end - pp[1])));
  }
  var A = b.a, B = b.b, n = stormChannel(M, 0, A[0], A[1], A[2], B[0], B[1], B[2], b.D, b.rough, b.seed, b.seed * 13 + pk, b.keep), vis = b.final || b.open;
  var nv = lead ? Math.max(2, Math.round(f * (n - 1)) + 1) : n, P = rb.P, W = rb.W, I = rb.I, wm = s * (b.final ? 0.3 : 0.17) * b.k * (lead ? 0.55 : 1);
  // Uneven brightness along the channel, in runs, as a real one has.
  stormSeed(b.seed * 7 + pk * 3 + 1);
  var cur = 0.8, tgt = 0.8;
  for (i = 0; i < nv; i++) {
    if ((i & 3) === 0) tgt = 0.55 + 0.45 * stormRnd();
    cur += (tgt - cur) * 0.45;
    // Inside the cloud the channel is half hidden: it brightens as it comes out of the wall.
    var u = i / (n - 1), inCloud = vis ? 1 : 0.3 + 0.7 * smooth(0.0, 0.35, u);
    P[i * 3] = M[i * 3]; P[i * 3 + 1] = M[i * 3 + 1]; P[i * 3 + 2] = M[i * 3 + 2];
    W[i] = wm * (0.75 + 0.35 * cur);
    I[i] = lv * cur * inCloud * (lead && i > nv - 3 ? 1.8 : 1) * gn;
  }
  rb.push(nv, lead ? 0.22 : (b.final ? 0.2 : 0.16), 0);
  for (k = 0; k < b.br.length; k++) {
    var br = b.br[k], j = Math.round(br.u * (n - 1));
    if (j >= nv - 1) continue;
    stormSeed(b.seed * 31 + k * 17 + pk);
    if (!lead && stormRnd() > 0.82) continue; // a branch can miss a re-stroke
    var rx = M[j * 3], ry = M[j * 3 + 1], rz = M[j * 3 + 2], dx, dy, dz, L;
    if (b.final) { dx = Math.cos(br.ang); dy = -br.droop; dz = Math.sin(br.ang); L = br.len; }
    else {
      var j2 = Math.min(n - 1, j + 6); dx = M[j2 * 3] - rx; dy = M[j2 * 3 + 1] - ry; dz = M[j2 * 3 + 2] - rz;
      var dl = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1; dx /= dl; dy /= dl; dz /= dl;
      var hx = -dz, hz = dx, hl = Math.sqrt(hx * hx + hz * hz) || 1, ca = Math.cos(br.ang), sa = Math.sin(br.ang) * br.side;
      dx = dx * ca + hx / hl * sa; dz = dz * ca + hz / hl * sa; dy = dy * ca - br.droop;
      var ex0 = B[0] - rx, ey0 = B[1] - ry, ez0 = B[2] - rz; L = Math.sqrt(ex0 * ex0 + ey0 * ey0 + ez0 * ez0) * br.len;
    }
    var dn = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1, LL = L * (lead ? clamp01((nv - 1 - j) / (n * 0.3)) : 1);
    dx /= dn; dy /= dn; dz /= dn;
    if (LL < s * 0.05) continue;
    var nb = stormChannel(P, 0, rx, ry, rz, rx + dx * LL, Math.max(0.1, ry + dy * LL), rz + dz * LL, hd ? 4 : 3, 0.26, b.seed * 5 + k, b.seed * 11 + k * 7 + pk, 1);
    var fade = vis ? 1 : 0.3 + 0.7 * smooth(0.0, 0.35, br.u);
    for (i = 0; i < nb; i++) { uu = i / (nb - 1); W[i] = s * (b.final ? 0.13 : 0.09) * b.k * (1 - 0.55 * uu); I[i] = lv * (0.62 - 0.47 * uu) * fade * gn; }
    var sj = Math.round((nb - 1) * 0.45) * 3, sx = P[sj], sy = P[sj + 1], sz = P[sj + 2];
    rb.push(nb, 0.2, 0.6);
    if (!br.sub) continue;
    // A twig off the branch, turned the other way (read from the branch before it is overwritten).
    var ta = -br.side * 0.6, cs = Math.cos(ta), sn = Math.sin(ta), tx = dx * cs - dz * sn, tz = dx * sn + dz * cs, SL = LL * 0.45;
    var ns = stormChannel(P, 0, sx, sy, sz, sx + tx * SL, Math.max(0.1, sy + (dy - 0.25) * SL), sz + tz * SL, hd ? 3 : 2, 0.28, b.seed * 3 + k, b.seed * 19 + k + pk, 1);
    for (i = 0; i < ns; i++) { uu = i / (ns - 1); W[i] = s * 0.065 * b.k * (1 - 0.5 * uu); I[i] = lv * (0.4 - 0.3 * uu) * fade * gn; }
    rb.push(ns, 0.22, 0.85);
  }
  return lead ? 0 : lv;
}

// The 20's own face: the number as light, and small arcs crackling off it (nArc of them, from seed).
var STORM_FA = new Float32Array(34);
function stormPaintFace(gf, col, k, nArc, seed) {
  var g2 = gf.g, lay = gf.lay, cv = gf.cv, FA = STORM_FA, i;
  gf.clear(); numberOnto(g2, lay, col);
  if (nArc > 0 && lay.strokes && lay.strokes.length) {
    stormSeed(seed); g2.lineCap = 'round'; g2.lineJoin = 'round';
    for (var a = 0; a < nArc; a++) {
      var sk = lay.strokes[Math.floor(stormRnd() * lay.strokes.length)], p = sk[Math.floor(stormRnd() * sk.length)];
      var an = stormRnd() * 6.283, ln = cv * (0.12 + stormRnd() * 0.22);
      FA[0] = p[0]; FA[1] = p[1]; FA[32] = p[0] + Math.cos(an) * ln; FA[33] = p[1] + Math.sin(an) * ln;
      for (var step = 16; step > 1; step >>= 1) for (i = 0; i < 16; i += step) {
        var m = (i + step / 2) * 2, ax = FA[i * 2], ay = FA[i * 2 + 1], bx = FA[(i + step) * 2], by = FA[(i + step) * 2 + 1], d = (stormRnd() - 0.5) * 0.55;
        FA[m] = (ax + bx) / 2 - (by - ay) * d; FA[m + 1] = (ay + by) / 2 + (bx - ax) * d;
      }
      g2.beginPath(); g2.moveTo(FA[0], FA[1]); for (i = 1; i < 17; i++) g2.lineTo(FA[i * 2], FA[i * 2 + 1]);
      g2.shadowColor = STORM_BLUE; g2.shadowBlur = cv * 0.03; g2.strokeStyle = STORM_BLUE; g2.lineWidth = cv * 0.014; g2.stroke();
      g2.shadowBlur = 0; g2.strokeStyle = '#ffffff'; g2.lineWidth = cv * 0.006; g2.stroke();
    }
  }
  gf.commit(k);
}
// Light only adds, so on a pale stone a pale number washes out: there it burns a deeper blue.
function stormNumColour(th) {
  var g0 = th.base && th.base.grad ? parseInt(th.base.grad[0].slice(1), 16) : 0;
  return (0.3 * (g0 >> 16 & 255) + 0.59 * (g0 >> 8 & 255) + 0.11 * (g0 & 255)) / 255 > 0.62 ? '#2f8fff' : STORM_NUM;
}

// Places to keep clear (o.clear: world x, z and radius of each other crit die), as the 8 nearest to
// `at`, each seen from the camera through a layer at height H: a die low on the page shows through a
// layer high above it displaced toward the middle of the view, so its clear spot sits there. Returns
// 8 vec3 (x, z relative to `at`, radius); radius 0 is an unused slot.
function stormSpots(st, at, clear, H, yd) {
  var T = st.T, cy = st.cam.position.y, f = (cy - H) / (cy - yd), out = [], k;
  var near = (clear || []).slice().sort(function (a, b) { return Math.hypot(a.x - at.x, a.z - at.z) - Math.hypot(b.x - at.x, b.z - at.z); });
  for (k = 0; k < 8; k++) out.push(near[k] ? new T.Vector3(near[k].x * f - at.x, near[k].z * f - at.z, near[k].r * f) : new T.Vector3(0, 0, 0));
  return out;
}
// The page's darkening patch, with a soft hole punched round each place to keep in view. Without any
// it is the engine's own dimmer.
function stormDimmer(st, at, radius, clear) {
  var m = dimmer(st, at, radius);
  if (!clear || !clear.length) return m;
  var spots = stormSpots(st, at, clear, 0, 0);
  spots.forEach(function (p) { p.x += at.x; p.y += at.z; });
  m.material.onBeforeCompile = function (sh) {
    sh.uniforms.uClr = { value: spots };
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nvarying vec2 vHw;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvHw = (modelMatrix * vec4(transformed, 1.0)).xz;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nuniform vec3 uClr[8]; varying vec2 vHw;')
      .replace('#include <color_fragment>', '#include <color_fragment>\nfor (int i = 0; i < 8; i++) { float cr = uClr[i].z; if (cr > 0.0) diffuseColor.a *= smoothstep(cr * 0.75, cr * 1.6, length(vHw - uClr[i].xy)); }');
  };
  return m;
}

// ---------- The cloud ----------
// One sheet above the die, shaded in the shader: big billows lit on the key light's side, finer churn
// over them, all turning round the eye (faster near it) and carried in, or thrown out, with the wall.
// Up to four lights inside it, for sheet lightning and the strikes. Normal blending, mid-toned, so it
// darkens a white page and still shows as grey-indigo over a dark one.
var STORM_NOISE = [
  'float sh(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float sn(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(sh(i), sh(i + vec2(1.0, 0.0)), u.x), mix(sh(i + vec2(0.0, 1.0)), sh(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float sf3(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < 3; i++) { s += a * sn(p); p = p * 2.07 + 1.3; a *= 0.5; } return s / 0.875; }',
  'float sfd(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < DETAIL; i++) { s += a * sn(p); p = p * 2.11 + 4.1; a *= 0.5; } return s; }'].join('\n');
var STORM_CLOUD_VS = 'uniform vec2 uC; varying vec2 vP; void main(){ vec4 w = modelMatrix * vec4(position, 1.0); vP = w.xz - uC; gl_Position = projectionMatrix * viewMatrix * w; }';
var STORM_CLOUD_FS = [
  'uniform float uTime, uIn, uRoll, uSpin, uDen, uOp; uniform vec2 uL; uniform vec4 uFl[4];',
  'uniform vec3 uDeep, uBody, uTop, uGlowA, uGlowB; uniform vec3 uClr[8]; varying vec2 vP;',
  STORM_NOISE,
  'vec2 sw(vec2 p, float r){ float a = uSpin * (0.3 + 1.5 / (1.0 + 0.3 * r)); float c = cos(a), s = sin(a); return vec2(c * p.x - s * p.y, s * p.x + c * p.y) * ((r + uRoll) / max(r, 0.01)); }',
  'float bil(vec2 p){ return sf3(sw(p, length(p)) * 0.42 + vec2(uTime * 0.04, 1.7)); }',
  'void main(){',
  ' float r = length(vP);',
  ' if (uOp <= 0.0 || r < uIn - 1.4 || r > uIn + 9.0) discard;',
  ' float b = bil(vP), e = 0.15;',
  ' vec2 g = vec2(bil(vP + vec2(e, 0.0)) - b, bil(vP + vec2(0.0, e)) - b) / e;',
  ' float dt = sfd(sw(vP, r) * 1.35 + vec2(-uTime * 0.09, uTime * 0.06));',
  ' float n = b * 0.72 + dt * 0.38;',
  // A clear eye round every die to keep in view, ragged like the grand die's own.
  ' float hole = 1.0;',
  ' for (int i = 0; i < 8; i++) { float cr = uClr[i].z; if (cr > 0.0) hole *= smoothstep(cr * 0.85, cr * 1.75, length(vP - uClr[i].xy) + (n - 0.5) * cr * 0.8); }',
  // The eye wall, ragged, and a soft outer edge, so the storm never covers the whole page.
  ' float wall = smoothstep(uIn - 0.3, uIn + 1.4, r + (n - 0.5) * 2.4);',
  ' float fade = 1.0 - smoothstep(uIn + 2.5, uIn + 7.5, r + (n - 0.5) * 3.0);',
  ' float d = clamp((n - 0.27) * 2.9, 0.0, 1.0) * wall * fade * uDen * hole;',
  ' if (d < 0.003) discard;',
  ' float lit = clamp(0.45 + dot(-g, uL) * 1.4, 0.0, 1.0);',
  ' vec3 col = mix(uDeep, uBody, smoothstep(0.2, 0.75, n));',
  ' col = mix(col, uTop, lit * smoothstep(0.35, 0.85, b) * 0.85);',
  // Lightning inside: it shows through the thin parts more than the thick.
  ' float gl = 0.0;',
  ' for (int i = 0; i < 4; i++) { vec2 q = vP - uFl[i].xy; gl += uFl[i].w * exp(-dot(q, q) / (uFl[i].z * uFl[i].z)); }',
  ' gl *= (0.75 + 0.5 * dt) * (1.15 - 0.35 * d);',
  ' vec3 gc = mix(uGlowA, uGlowB, smoothstep(0.3, 1.2, gl));',
  // Lit, the cloud keeps its billows: the light picks out the folds rather than painting over them.
  ' col = mix(col, gc * (0.12 + 1.1 * smoothstep(0.3, 0.78, n) + 0.3 * lit), clamp(gl * 0.8, 0.0, 0.95));',
  ' gl_FragColor = vec4(col, clamp(d * 1.35, 0.0, 0.93) * uOp);',
  ' #include <tonemapping_fragment>',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function stormCloud(st, at, H, R, clr) {
  var T = st.T, fl = [0, 1, 2, 3].map(function () { return new T.Vector4(0, 0, 1, 0); });
  // Toward the key light, flat on the page: it sits up and to the left of the page.
  var lx = -st.hw, lz = -st.hh, ll = Math.sqrt(lx * lx + lz * lz);
  var uni = { uC: { value: new T.Vector2(at.x, at.z) }, uTime: { value: 0 }, uIn: { value: 40 }, uRoll: { value: 0 }, uSpin: { value: 0 }, uDen: { value: 0 }, uOp: { value: 1 },
    uL: { value: new T.Vector2(lx / ll, lz / ll) }, uFl: { value: fl }, uClr: { value: clr },
    uDeep: { value: new T.Color('#14151c') }, uBody: { value: new T.Color('#353844') }, uTop: { value: new T.Color('#7a8094') },
    uGlowA: { value: new T.Color('#8a90ff') }, uGlowB: { value: new T.Color('#f0f8ff') } };
  var mat = new T.ShaderMaterial({ uniforms: uni, vertexShader: STORM_CLOUD_VS, fragmentShader: '#define DETAIL 3\n' + STORM_CLOUD_FS, transparent: true, depthWrite: false });
  var m = new T.Mesh(new T.PlaneGeometry(R * 2, R * 2), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, H, at.z); m.renderOrder = 5; m.frustumCulled = false;
  return { mesh: m, uni: uni, fl: fl };
}

// ---------- The scorch ----------
// A Lichtenberg figure: branching fern-like burns out from the die, drawn once into textures, one
// colour channel per strike (the three side strikes in one texture, the last in another). A spot
// lights when that strike's discharge has run out as far as it, then cools from white-blue through
// violet to a faint dark mark. Keeping strikes in separate channels matters: the blurred mip levels
// that give the glow would otherwise mix a lit burn with one still to come.
var STORM_SCORCH_FS = [
  'uniform sampler2D uA, uB; uniform vec4 uT; uniform float uTime, uFade, uSpan, uR0, uSpeed, uB1, uB2; uniform vec3 uHot, uBlue, uViolet, uMark; varying vec2 vUv;',
  'vec3 heat(float h){ return mix(mix(uViolet, uBlue, smoothstep(0.06, 0.4, h)), uHot, smoothstep(0.55, 0.95, h)); }',
  'void main(){',
  ' vec4 c0 = vec4(texture2D(uA, vUv).rgb, texture2D(uB, vUv).r);',
  ' vec4 c1 = vec4(texture2D(uA, vUv, uB1).rgb, texture2D(uB, vUv, uB1).r);',
  ' vec4 c2 = vec4(texture2D(uA, vUv, uB2).rgb, texture2D(uB, vUv, uB2).r);',
  ' float dist = max(0.0, length(vUv - 0.5) * uSpan - uR0);',
  ' vec4 age = uTime - uT - dist / uSpeed;',
  ' vec4 lit = smoothstep(0.0, 0.03, age), h = exp(-max(age, 0.0) * 3.0);',
  ' vec4 k = lit * (c0 * (0.12 + 2.8 * h) + c1 * h * 1.4 + c2 * h * 0.9);',
  ' vec3 glow = heat(h.x) * k.x + heat(h.y) * k.y + heat(h.z) * k.z + heat(h.w) * k.w;',
  ' vec4 mk = lit * (c0 * smoothstep(0.05, 0.6, age) * 0.6 + c1 * smoothstep(0.05, 0.8, age) * 0.12);',
  ' float a = clamp(mk.x + mk.y + mk.z + mk.w, 0.0, 0.7) * uFade;',
  ' gl_FragColor = vec4(uMark * a + glow * uFade, a);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function stormScorch(st, at, s, arms, hd, times) {
  var T = st.T, S = hd ? 1024 : 512, Wd = s * 11, R0 = s * 0.7, rnd = mulberry(913), step = s * 0.055, budget = hd ? 7000 : 3500;
  var ca = canvas(S), cb = canvas(S), sc = canvas(S), ga = ca.getContext('2d'), gb = cb.getContext('2d'), g = sc.getContext('2d');
  [ga, gb].forEach(function (c) { c.fillStyle = '#000'; c.fillRect(0, 0, S, S); c.globalCompositeOperation = 'lighter'; });
  g.lineCap = 'round'; g.lineJoin = 'round';
  function px(v) { return (v / Wd + 0.5) * S; }
  function flush(run, w) {
    g.lineWidth = Math.max(1, w * S / Wd);
    g.beginPath(); g.moveTo(run[0], run[1]); for (var i = 2; i < run.length; i += 2) g.lineTo(run[i], run[i + 1]); g.stroke();
  }
  // One branch: a jagged walk that keeps to its heading, forking now and then into thinner, shorter ones.
  function grow(x, z, h0, L, w, depth) {
    var h = h0, n = Math.max(2, Math.round(L / step)), run = [px(x), px(z)], wr = w;
    for (var i = 1; i <= n && budget > 0; i++, budget--) {
      h += (rnd() - 0.5) * 0.9 + (h0 - h) * 0.2;
      x += Math.cos(h) * step; z += Math.sin(h) * step;
      run.push(px(x), px(z));
      var ww = w * (1 - 0.65 * i / n);
      if (run.length >= 10 || i === n) { flush(run, wr); run = [run[run.length - 2], run[run.length - 1]]; wr = ww; }
      if (depth < 5 && i < n - 2 && rnd() < 0.1 + depth * 0.03)
        grow(x, z, h + (rnd() < 0.5 ? -1 : 1) * (0.4 + rnd() * 0.55), (n - i) * step * (0.3 + rnd() * 0.4) + step * 2, ww * 0.6, depth + 1);
    }
  }
  for (var k = 0; k < 4; k++) {
    g.globalCompositeOperation = 'source-over'; g.fillStyle = '#000'; g.fillRect(0, 0, S, S);
    g.strokeStyle = ['#ff0000', '#00ff00', '#0000ff', '#ff0000'][k];
    arms.forEach(function (a) { if (a.k === k) grow(Math.cos(a.a) * R0, Math.sin(a.a) * R0, a.a, a.L, a.w, 0); });
    (k < 3 ? ga : gb).drawImage(sc, 0, 0);
  }
  var ta = new T.CanvasTexture(ca), tb = new T.CanvasTexture(cb);
  var uni = { uA: { value: ta }, uB: { value: tb }, uT: { value: new T.Vector4(times[0], times[1], times[2], times[3]) }, uTime: { value: 0 }, uFade: { value: 1 },
    uSpan: { value: Wd }, uR0: { value: R0 }, uSpeed: { value: s * 20 },
    // Blurred mip levels for the glow; a smaller texture needs a smaller step or the blur turns blocky.
    uB1: { value: hd ? 2.0 : 1.4 }, uB2: { value: hd ? 3.6 : 2.8 },
    uHot: { value: new T.Color(STORM_WHITE) }, uBlue: { value: new T.Color(STORM_BLUE) }, uViolet: { value: new T.Color(STORM_VIOLET) }, uMark: { value: new T.Color('#1a1522') } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    vertexShader: 'varying vec2 vUv; void main(){ vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }', fragmentShader: STORM_SCORCH_FS });
  var m = new T.Mesh(new T.PlaneGeometry(Wd, Wd), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.008, at.z); m.renderOrder = 0;
  return { mesh: m, uni: uni, dispose: function () { ta.dispose(); tb.dispose(); } };
}

// ---------- The shockwave ----------
// A thin bright crest with a soft band of dark behind it, as if it pushes dust ahead over the page: the
// crest shows on a dark page, the band on a white one. Premultiplied, like the bolts.
var STORM_RING_FS = [
  'uniform float uR, uW, uA; uniform vec3 uHot, uDust, uGlow; varying vec2 vP;',
  'void main(){',
  ' float an = atan(vP.y, vP.x);',
  ' float d = length(vP) - uR * (1.0 + 0.012 * sin(an * 7.0 + 1.3) + 0.008 * sin(an * 13.0 - 0.7));',
  ' float crest = exp(-d * d / (uW * uW * 0.02));',
  ' float band = smoothstep(-uW, -uW * 0.25, d) * (1.0 - smoothstep(-uW * 0.25, uW * 0.06, d));',
  ' gl_FragColor = vec4((uDust * band * 0.28 + uGlow * band * 0.1 + uHot * crest * 0.75) * uA, (band * 0.28 + crest * 0.14) * uA);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function stormRing(st, at, maxR) {
  var T = st.T, uni = { uR: { value: 0 }, uW: { value: 1 }, uA: { value: 0 }, uHot: { value: new T.Color(STORM_WHITE) }, uDust: { value: new T.Color('#141322') }, uGlow: { value: new T.Color(STORM_VIOLET) } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    vertexShader: 'varying vec2 vP; void main(){ vP = position.xy; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }', fragmentShader: STORM_RING_FS });
  var m = new T.Mesh(new T.PlaneGeometry(maxR * 2.3, maxR * 2.3), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.03, at.z); m.renderOrder = 2; m.visible = false;
  return { mesh: m, uni: uni };
}

// The die's corners and edges, read from its face drawings, for arcs to crawl along. A die whose faces
// carry no outline falls back to its face centres, each joined to its three nearest.
function stormFrame(die) {
  var s = die.size, V = [], F = [], adj = [];
  function vid(v) { for (var j = 0; j < V.length; j++) if (V[j].distanceToSquared(v) < 1e-4 * s * s) return j; V.push(v); adj.push([]); return V.length - 1; }
  function link(a, b) { if (a !== b && adj[a].indexOf(b) < 0) { adj[a].push(b); adj[b].push(a); } }
  die.faces.forEach(function (fc, i) {
    var poly = fc.lay && fc.lay.poly;
    if (!poly || !poly.length) { F.push(null); return; }
    var ids = poly.map(function (p) { return vid(faceToLocal(die, i, p[0], p[1])); });
    ids.forEach(function (id, k) { link(id, ids[(k + 1) % ids.length]); });
    F.push(ids);
  });
  if (V.length < 4) {
    V = []; adj = []; F = die.faces.map(function () { return null; });
    die.faces.forEach(function (fc) { V.push(fc.c.clone().multiplyScalar(1.08)); adj.push([]); });
    V.forEach(function (v, i) {
      var near = V.map(function (w, j) { return [w.distanceToSquared(v), j]; }).sort(function (a, b) { return a[0] - b[0]; });
      for (var k = 1; k <= 3 && k < near.length; k++) link(i, near[k][1]);
    });
  }
  return { V: V, F: F, adj: adj };
}

function fxStorm(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, frame = 0;
  // A new storm every time: where the bolts come from, their shapes and the scorch all follow this seed.
  var rnd = mulberry(7411 + Math.floor(Math.random() * 1e6));
  g.updateMatrixWorld(true);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), at = new T.Vector3(p0.x, 0, p0.z), top = worldOf(die, die.faces[fi].c);
  var END = 4.8, TF = 2.85, TS = [1.45, 1.9, 2.3], HC = s * 4.1, EYE = s * 3.0;
  // The wall starts past the canvas corner farthest from the die, so the cloud rolls in from off the page.
  var far = Math.sqrt(Math.pow(st.hw + Math.abs(at.x), 2) + Math.pow(st.hh + Math.abs(at.z), 2)) * 1.05;
  function half(n) { return hd ? n : Math.ceil(n / 2); }
  var NUM = stormNumColour(th);

  // The die's frame in world space, refreshed each frame because the die shakes and hops.
  var fr = stormFrame(die), WV = fr.V.map(function (v) { return v.clone().applyMatrix4(g.matrixWorld); });
  var FN = die.faces.map(function (fc) { return fc.n.clone().applyQuaternion(g.quaternion); });
  var topV = fr.F[fi] || [fi].concat(fr.adj[fi]);
  function refreshFrame() { g.updateMatrixWorld(true); for (var i = 0; i < WV.length; i++) WV[i].copy(fr.V[i]).applyMatrix4(g.matrixWorld); }

  // ----- The bolts, planned up front: three from the cloud wall, then the big one from overhead -----
  // A bolt from (ox, oz) finds a corner that is high and on its side of the die, as lightning finds points.
  function hitFrom(ox, oz) {
    var best = 0, bs = -1e9, dx = ox - p0.x, dz = oz - p0.z, dl = Math.sqrt(dx * dx + dz * dz) || 1;
    WV.forEach(function (v, i) {
      var hx = v.x - p0.x, hz = v.z - p0.z, hl = Math.sqrt(hx * hx + hz * hz) || 1;
      var sc = (hx * dx + hz * dz) / (hl * dl) * 0.6 * Math.min(1, hl / s * 2) + (v.y - p0.y) / s;
      if (sc > bs) { bs = sc; best = i; }
    });
    return WV[best];
  }
  var bolts = [], a0 = rnd() * 6.283;
  TS.forEach(function (ts, k) {
    var a = a0 + k * 2.2 + (rnd() - 0.5) * 0.7, ro = EYE + s * (1.9 + rnd() * 0.7), ox = at.x + Math.cos(a) * ro, oz = at.z + Math.sin(a) * ro;
    bolts.push(stormPlan(rnd, s, hd, ts, ox, HC - s * 0.15, oz, hitFrom(ox, oz), false, 100 + k * 37));
  });
  bolts.push(stormPlan(rnd, s, hd, TF, at.x + s * 0.25, s * 12, at.z - s * 0.15, top, true, 300));

  // The scorch: a few arms toward each side bolt's corner, and a full ring for the last strike.
  var arms = [];
  bolts.forEach(function (b, k) {
    var base = Math.atan2(b.b[2] - p0.z, b.b[0] - p0.x), na = b.final ? (hd ? 11 : 9) : 3 + (rnd() < 0.5 ? 1 : 0);
    for (var i = 0; i < na; i++) arms.push({ a: b.final ? i / na * 6.283 + (rnd() - 0.5) * 0.45 : base + (rnd() - 0.5) * 2.4, k: k,
      L: s * (b.final ? 2.5 + rnd() * 1.5 : 1.3 + rnd() * 1.1), w: s * (b.final ? 0.05 : 0.04) });
  });

  // ----- What goes in the scene -----
  var clear = o && o.clear || [];
  var dim = stormDimmer(st, at, s * 9.5, clear);
  var cloud = stormCloud(st, at, HC, far * 1.3 + s * 6, stormSpots(st, at, clear, HC, s * 0.9)); st.scene.add(cloud.mesh);
  var scorch = stormScorch(st, at, s, arms, hd, bolts.map(function (b) { return b.t0 + 0.01; })); st.scene.add(scorch.mesh);
  var rb = stormRibbons(st, hd ? 2600 : 1500); st.scene.add(rb.mesh);
  // Light on the page round the die (it shows on a dark page; on a white one the dimmer lifting is the flash).
  var pageFlash = glow(st, spriteTex(T), STORM_BLUE, s * 11); pageFlash.position.set(at.x, 0.08, at.z); pageFlash.renderOrder = 1;
  // Centred inside the die, so the die hides its middle and it reads as a charged rim of light.
  var halo = glow(st, spriteTex(T), STORM_BLUE, s * 4.2); halo.position.copy(p0);
  var hot = glow(st, spriteTex(T), STORM_WHITE, s * 1.6); hot.renderOrder = 8;
  var star = glow(st, starTex(T), '#ffffff', s * 8); star.position.set(top.x, top.y + s * 0.6, top.z); star.renderOrder = 9;
  var ring = stormRing(st, at, s * 8);
  [pageFlash, halo, hot, star, ring.mesh].forEach(function (o) { st.scene.add(o); });
  var fades = [dim, pageFlash, halo, hot, star];
  var lightA = new T.PointLight(0xcfe9ff, 0, s * 12, 1.3); lightA.position.set(p0.x, top.y + s * 1.6, p0.z); st.scene.add(lightA);
  // Basic keeps to one extra light; HD adds one along the incoming bolt, so the die is lit from its side.
  var lightB = hd ? new T.PointLight(0xa9c4ff, 0, s * 14, 1.1) : null; if (lightB) st.scene.add(lightB);

  // ----- Arcs crawling over the die -----
  var NA = hd ? 10 : 6, arcs = [], i;
  for (i = 0; i < NA; i++) arcs.push({ on: false, kind: 0, path: [0, 0, 0, 0], np: 0, f: 0, k1: 0, k2: 0, ta: 0, tb: 0, ga: 1.5, life: 0, age: 0, seed: 0 });
  function isUpper(i) { return WV[i].y > g.position.y - s * 0.12; }
  function pick(test) { for (var k = 0; k < 12; k++) { var i = Math.floor(rnd() * WV.length); if (test(i)) return i; } return -1; }
  function spawnArc(a, onTop) {
    a.on = false; a.age = 0; a.life = 0.06 + rnd() * 0.12; a.seed = Math.floor(rnd() * 1e6);
    var r = rnd(), v, k, hasF = !!fr.F[onTop ? fi : 0];
    a.kind = onTop ? (r < 0.55 ? 0 : 1) : (r < 0.55 ? 0 : r < 0.85 ? 1 : 2);
    if (a.kind === 1 && !hasF) a.kind = 0;
    if (a.kind === 0) {
      // Along the rims: from a corner, one to three edges on.
      v = onTop ? topV[Math.floor(rnd() * topV.length)] : pick(isUpper); if (v < 0) return;
      a.path[0] = v; a.np = 1;
      var steps = 1 + Math.floor(rnd() * (onTop ? 2 : 3)), prev = -1;
      for (k = 0; k < steps; k++) {
        var nb = fr.adj[v], c = -1;
        for (var tr = 0; tr < 6 && nb.length; tr++) { var q = nb[Math.floor(rnd() * nb.length)]; if (q !== prev && (onTop ? topV.indexOf(q) >= 0 : isUpper(q))) { c = q; break; } }
        if (c < 0) break;
        prev = v; v = c; a.path[a.np++] = v;
      }
      a.on = a.np > 1;
    } else if (a.kind === 1) {
      // Across a face, edge to edge.
      var f = onTop ? fi : -1;
      if (f < 0) for (k = 0; k < 12; k++) { var ff = Math.floor(rnd() * FN.length); if (FN[ff].y > 0.1 && fr.F[ff]) { f = ff; break; } }
      if (f < 0) return;
      var L = fr.F[f].length; a.f = f; a.k1 = Math.floor(rnd() * L); a.k2 = (a.k1 + 1 + Math.floor(rnd() * (L - 1))) % L;
      a.ta = 0.15 + rnd() * 0.7; a.tb = 0.15 + rnd() * 0.7; a.on = true;
    } else {
      v = pick(function (i) { var dy = WV[i].y - g.position.y; return dy < s * 0.35 && dy > -s * 0.6; }); if (v < 0) return;
      a.path[0] = v; a.ga = 1.3 + rnd() * 0.45; a.life *= 0.7; a.on = true;
    }
  }
  function drawArc(a, lvl) {
    var P = rb.P, W = rb.W, I = rb.I, n, i, c = g.position, sd = a.seed + Math.floor(frame / 2) * 7919, D = hd ? 3 : 2, seg = 1 << D;
    if (a.kind === 0) {
      for (var k = 0; k < a.np - 1; k++) { var A = WV[a.path[k]], B = WV[a.path[k + 1]]; stormChannel(P, k * seg, A.x, A.y, A.z, B.x, B.y, B.z, D, 0.16, sd + k, sd + k + 1, 0); }
      n = (a.np - 1) * seg + 1;
      // Just off the rim, out from the centre, so it crawls over the metal rather than inside it.
      for (i = 0; i < n * 3; i += 3) { P[i] = c.x + (P[i] - c.x) * 1.035; P[i + 1] = c.y + (P[i + 1] - c.y) * 1.035; P[i + 2] = c.z + (P[i + 2] - c.z) * 1.035; }
    } else if (a.kind === 1) {
      var Fv = fr.F[a.f], L = Fv.length, A0 = WV[Fv[a.k1]], A1 = WV[Fv[(a.k1 + 1) % L]], B0 = WV[Fv[a.k2]], B1 = WV[Fv[(a.k2 + 1) % L]], nf = FN[a.f], lift = s * 0.025;
      n = stormChannel(P, 0, A0.x + (A1.x - A0.x) * a.ta + nf.x * lift, A0.y + (A1.y - A0.y) * a.ta + nf.y * lift, A0.z + (A1.z - A0.z) * a.ta + nf.z * lift,
        B0.x + (B1.x - B0.x) * a.tb + nf.x * lift, B0.y + (B1.y - B0.y) * a.tb + nf.y * lift, B0.z + (B1.z - B0.z) * a.tb + nf.z * lift, D, 0.2, sd, sd + 1, 0);
    } else {
      // A spark that jumps from a corner down to the page.
      var V = WV[a.path[0]], hx = V.x - c.x, hz = V.z - c.z, hl = Math.sqrt(hx * hx + hz * hz) || 1;
      n = stormChannel(P, 0, V.x, V.y, V.z, c.x + hx / hl * s * a.ga, 0.03, c.z + hz / hl * s * a.ga, D + 1, 0.22, sd, sd + 1, 0);
    }
    stormSeed(sd + 5);
    var base = lvl * Math.min(1, 2 * Math.sin(Math.PI * clamp01(a.age / a.life))) * (0.6 + 0.4 * stormRnd());
    for (i = 0; i < n; i++) { var u = i / (n - 1); W[i] = s * 0.05 * (0.8 + 0.4 * stormRnd()); I[i] = base * (0.55 + 0.45 * stormRnd()) * Math.min(1, Math.sin(Math.PI * u) * 3 + 0.3); }
    rb.push(n, 0.24, 0.3);
  }

  var gf = glowFace(die, fi);
  function paintFace(col, k, nArc, seed) { stormPaintFace(gf, col, k, nArc, seed); }

  // ----- Strikes -----
  var jolt = 0;
  function strike(b) {
    var hx = b.b[0], hy = b.b[1], hz = b.b[2];
    hot.position.set(hx, hy + s * 0.35, hz);
    st.fx.push(spray(st, { n: half(b.final ? 90 : 34), colors: [STORM_WHITE, '#ffffff', STORM_BLUE], size: s * 0.07, life: [0.2, 0.55], drag: 1.6, g: 11,
      spawn: function (i, r) { var a = r() * 6.283, v = (b.final ? 6 : 4) * (0.35 + r() * 0.8); return { p: [hx, hy, hz], v: [Math.cos(a) * v, 1 + r() * 3.5, Math.sin(a) * v] }; } }));
    if (!b.final) { jolt = 0.12; return; }
    // The big one: the shockwave (drawn in update) and dust thrown out along the page.
    ring.mesh.visible = true;
    st.fx.push(spray(st, { n: half(28), colors: ['#3a3d4c', '#2a2c38', '#4e5264'], size: s * 1.1, life: [0.7, 1.2], drag: 2.4, g: -0.2, blend: 'normal', smoke: true, alpha: 0.38,
      spawn: function (i, r) { var a = r() * 6.283, v = 5 + r() * 3.5; return { p: [at.x + Math.cos(a) * s * 0.9, s * 0.25, at.z + Math.sin(a) * s * 0.9], v: [Math.cos(a) * v, 0.3 + r() * 0.6, Math.sin(a) * v] }; } }));
  }
  function sparksAt(v, n, life) {
    var x = v.x, y = v.y, z = v.z;
    st.fx.push(spray(st, { n: half(n), colors: [STORM_WHITE, STORM_BLUE], size: s * 0.055, life: life, drag: 2, g: 9,
      spawn: function (i, r) { var a = r() * 6.283, sp = 1.2 + r() * 2.2; return { p: [x, y, z], v: [Math.cos(a) * sp, 0.6 + r() * 2, Math.sin(a) * sp] }; } }));
  }

  // Sheet lightning: flashes that light patches of the cloud from inside, between the strikes.
  var sheets = [{ t: 0.55, d: 0.2, on: false }, { t: 0.85, d: 0.2, on: false }];
  var spin = 0, fz = 0.5, sparkT = 0, fl = cloud.fl;
  return { update: function (dt) {
    t += dt; frame++;
    if (t >= END) {
      drop(st, [dim, cloud.mesh, scorch.mesh, rb.mesh, pageFlash, halo, hot, star, ring.mesh, lightA, lightB]); scorch.dispose();
      g.position.copy(p0); g.quaternion.copy(q0); g.updateMatrixWorld(true);
      shadeDie(die, 1); tintDie(die, th.burn, 0); glowNumeral(die, fi, NUM);
      return false;
    }
    if (!(frame & 1)) fz = rnd();
    var storm = smooth(0, 1.2, t) * (1 - smooth(3.4, 4.5, t));
    var charge = smooth(0.45, 1.4, t) * (1 - smooth(TF, TF + 0.35, t));
    var gather = t < TF ? smooth(2.45, TF, t) : 0;
    var after = t < TF ? 0 : 1 - smooth(TF + 0.4, 4.2, t);

    // The die: a shudder on each side strike, a hop on the last.
    var jx = 0, jy = 0, jz = 0;
    if (jolt > 0) { jolt -= dt; var ja = s * 0.035 * Math.max(0, jolt / 0.12); jx = (rnd() - 0.5) * ja; jz = (rnd() - 0.5) * ja; jy = rnd() * ja * 0.3; }
    var hk = (t - TF) / 0.34; if (hk > 0 && hk < 1) jy += s * 0.16 * Math.sin(hk * Math.PI);
    g.position.set(p0.x + jx, p0.y + jy, p0.z + jz); refreshFrame();

    // Lightning and arcs, all into the one ribbon mesh.
    rb.begin();
    var litL = 0, litF = 0, envL = 0, envF = 0, lastL = null;
    for (i = 0; i < bolts.length; i++) {
      var b = bolts[i], tau = t - b.t0, lv = stormBolt(rb, s, b, tau);
      if (!b.struck && tau >= 0) { b.struck = true; strike(b); }
      var e = tau >= 0 ? Math.exp(-tau * (b.final ? 6 : 8)) : 0;
      if (b.final) { litF = lv; envF = e; } else { if (e > envL) { envL = e; lastL = b; } litL = Math.max(litL, lv); }
    }
    var onTop = t >= TF, want, lvl;
    if (!onTop) { want = Math.round(NA * smooth(0.5, 1.5, t) * (0.6 + 0.4 * gather)); lvl = 0.75 + 0.6 * gather + litL * 0.8; }
    else { var ak = 1 - smooth(TF + 0.3, 4.1, t); want = ak > 0.02 ? Math.max(1, Math.round((hd ? 5 : 3) * ak)) : 0; lvl = 0.6 + 0.4 * ak + litF; }
    var active = 0;
    for (i = 0; i < NA; i++) { var a = arcs[i]; if (a.on) { a.age += dt; if (a.age >= a.life) a.on = false; } if (a.on) active++; }
    for (i = 0; i < NA && active < want; i++) if (!arcs[i].on && rnd() < 0.6) { spawnArc(arcs[i], onTop); if (arcs[i].on) active++; }
    for (i = 0; i < NA; i++) if (arcs[i].on) drawArc(arcs[i], lvl);
    rb.end();

    // Small sparks jumping off the charged corners, and later off the 20.
    sparkT -= dt;
    if (sparkT <= 0 && ((t > 0.8 && t < TF) || (t > TF + 0.4 && t < 3.8))) {
      sparkT = 0.14 + rnd() * 0.16;
      var vi = onTop ? topV[Math.floor(rnd() * topV.length)] : pick(isUpper);
      if (vi >= 0) sparksAt(WV[vi], 8, [0.15, 0.35]);
    }

    // The cloud: rolls in, tightens round the eye before the last strike, then is thrown back and thins.
    var U = cloud.uni, inn;
    if (t < TF) inn = far + (EYE - far) * out3(t / 1.7) - s * 0.35 * gather;
    else inn = EYE - s * 0.35 + s * 2 * out3((t - TF) / 0.6) + far * 0.32 * smooth(TF + 0.3, 4.5, t);
    spin += dt * (0.16 + 0.22 * Math.max(charge, after));
    // The billows ride in with the wall at half its speed: all of it would smear them into streaks.
    U.uTime.value = t; U.uIn.value = inn; U.uRoll.value = (inn - EYE) * 0.5; U.uSpin.value = spin;
    U.uDen.value = smooth(0, 1.0, t) * (1 - 0.5 * smooth(TF + 0.2, 4.2, t));
    U.uOp.value = 1 - smooth(3.45, 4.45, t);
    var amp = 0.9 * smooth(0.4, 0.8, t) * (1 - smooth(TF + 0.3, TF + 1.3, t));
    for (i = 0; i < 2; i++) {
      var sh = sheets[i];
      if (!sh.on && t >= sh.t) { var ang = rnd() * 6.283, rr = inn + s * (1.0 + rnd() * 2.6); fl[i].set(Math.cos(ang) * rr, Math.sin(ang) * rr, s * (1.5 + rnd() * 1.3), 0); sh.d = 0.14 + rnd() * 0.12; sh.on = true; }
      var sk = sh.on ? (t - sh.t) / sh.d : 0;
      if (sh.on && sk >= 1) { sh.on = false; sh.t = t + 0.12 + rnd() * 0.5; sk = 0; }
      fl[i].w = sh.on ? amp * Math.sin(sk * Math.PI) * (0.55 + 0.45 * Math.sin(sk * Math.PI * 3)) : 0;
    }
    if (envF > 0) fl[2].set(0, 0, EYE * 1.7, 1.8 * envF);
    else if (lastL) fl[2].set(lastL.a[0] - at.x, lastL.a[2] - at.z, s * 2.8, 1.1 * envL + 0.3 * litL);
    else fl[2].w = 0;
    fl[3].set(0, 0, inn + s * 1.5, 0.18 * charge * (0.8 + 0.2 * fz) + 0.6 * gather * (0.7 + 0.3 * fz) + 0.2 * after * fz);

    // The page: darker under the storm; each strike lifts the dark for an instant, which is the flash.
    var big = Math.max(envL * 0.6, envF);
    dim.material.opacity = 0.64 * storm * (1 - 0.4 * big);
    pageFlash.material.opacity = 0.2 * big;
    halo.material.opacity = 0.14 * charge * (0.6 + 0.4 * fz) + 0.12 * gather + 0.3 * Math.max(litL, litF) + 0.1 * after;
    hot.material.opacity = Math.min(1, Math.max(litL * 0.8, litF * 0.7)); hot.scale.setScalar(s * (1.6 + 2.4 * envF));
    var sk2 = Math.exp(-Math.max(0, t - TF) * 7) * (t >= TF ? 1 : 0);
    star.material.opacity = 0.75 * sk2; star.material.rotation = Math.max(0, t - TF) * 0.5; star.scale.setScalar(s * 8 * (0.7 + 0.5 * (1 - sk2)));
    if (ring.mesh.visible) { var rk = clamp01((t - TF) / 0.85); ring.uni.uR.value = s * (0.8 + 7.2 * out3(rk)); ring.uni.uW.value = s * (0.4 + 1.1 * rk); ring.uni.uA.value = (1 - rk) * (1 - rk); ring.mesh.visible = rk < 1; }
    scorch.uni.uTime.value = t; scorch.uni.uFade.value = 1 - smooth(3.6, 4.65, t);
    // Layers with nothing to show are skipped, so they cost no draw call.
    for (i = 0; i < fades.length; i++) fades[i].visible = fades[i].material.opacity > 0.003;
    cloud.mesh.visible = U.uOp.value > 0 && U.uDen.value > 0.003; scorch.mesh.visible = t > TS[0] && t < 4.7;

    // Light on the die from the storm.
    lightA.intensity = charge * (4 + 5 * fz) + gather * 6 * fz + litL * 45 + litF * 160 + after * 3 * fz;
    if (lightB) {
      if (litF > 0) { lightB.position.set(top.x, top.y + s * 3, top.z); lightB.intensity = litF * 120; }
      else if (lastL && litL > 0) { var A = lastL.a, B = lastL.b; lightB.position.set(B[0] + (A[0] - B[0]) * 0.25, B[1] + (A[1] - B[1]) * 0.25, B[2] + (A[2] - B[2]) * 0.25); lightB.intensity = litL * 60; }
      else lightB.intensity = 0;
    }
    shadeDie(die, 1 - 0.3 * storm + 0.25 * Math.max(litL * 0.7, litF));
    tintDie(die, STORM_BLUE, charge * (0.3 + 0.5 * fz) + gather * 0.6 * fz + litL * 2.2 + litF * 3.5 + after * 0.55 * fz);
    var flick = 0.8 + 0.2 * fz, hit = litL + litF > 0.05;
    if (frame % (hit ? 2 : 3) === 0) {
      var nArc = !onTop ? Math.round(charge * (hd ? 3 : 2) + gather) : Math.round(after * (hd ? 3 : 2));
      paintFace(mix(th.burn, NUM, smooth(0.4, 1.3, t)), 1.6 + charge * 1.3 * flick + gather * 0.8 * fz + litL * 3 + litF * 5 + after * 0.9 * flick, nArc, frame * 31 + 7);
    }
    return true;
  } };
}

// ---------- Echo: a bolt jumps over ----------
// The grand die's last strike throws one forked bolt across the page to this die (or, with no grand die
// to come from, one drops onto it at a slant), a flash on arrival, and a crown of sparks crackles round
// the 20 for a moment before it settles. Everything draws above the grand's clouds (render order 60 up,
// no depth test), so the bolt and the 20 stay in view whatever the storm is doing to the page.
function fxStormEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, frame = 0, END = 1.75, i;
  var rnd = mulberry(5113 + Math.floor(Math.random() * 1e6)), NUM = stormNumColour(th);
  g.updateMatrixWorld(true);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), at = new T.Vector3(p0.x, 0, p0.z), fc = die.faces[fi];
  var top = worldOf(die, fc.c), nrm = fc.n.clone().applyQuaternion(g.quaternion).normalize(), poly = fc.lay && fc.lay.poly;
  var corners = poly && poly.length ? poly.map(function (p) { return worldOf(die, faceToLocal(die, fi, p[0], p[1])); }) : [];
  if (corners.length < 3) { corners = []; for (i = 0; i < 3; i++) corners.push(top.clone().add(new T.Vector3(Math.cos(i * 2.094), 0, Math.sin(i * 2.094)).multiplyScalar(s * 0.8))); }

  // Where the bolt comes from: the grand die's top, or, alone, a slant from above on the side facing the page's middle.
  var src;
  if (e && e.from) { src = worldOf(e.from, e.from.faces[e.fromFi].c); src.y += s * 0.15; }
  else { var an = Math.atan2(-at.z, -at.x) + (rnd() - 0.5) * 1.2, HH = s * 8; src = new T.Vector3(top.x + Math.cos(an) * HH * 0.45, HH, top.z + Math.sin(an) * HH * 0.45); }
  // It lands on the corner of the top face that looks toward it, as lightning finds points.
  var hit = corners[0], bs = -1e9, dx = src.x - top.x, dz = src.z - top.z, dl = Math.sqrt(dx * dx + dz * dz) || 1;
  corners.forEach(function (c) { var sc = ((c.x - top.x) * dx + (c.z - top.z) * dz) / dl; if (sc > bs) { bs = sc; hit = c; } });
  var bolt = stormPlan(rnd, s, hd, 0.12, src.x, src.y, src.z, hit, false, 700 + Math.floor(rnd() * 1e5));
  bolt.open = true; bolt.k = 1.3; bolt.gain = 1.3; bolt.br.length = hd ? 5 : 3; bolt.br.forEach(function (b) { b.len *= 0.7; });

  // The crown: a ring of jagged arcs round the top face and short spikes thrown off it.
  var U = corners[0].clone().sub(top).projectOnPlane(nrm).normalize(), V = new T.Vector3().crossVectors(nrm, U), Rr = 0;
  corners.forEach(function (c) { Rr += c.distanceTo(top) / corners.length; }); Rr *= 1.1;

  var rb = stormRibbons(st, hd ? 900 : 560, 60, 1); st.scene.add(rb.mesh);
  var edim = dimmer(st, at, s * 2.7), hotA = glow(st, spriteTex(T), STORM_WHITE, s * 1.6), hotB = glow(st, spriteTex(T), STORM_WHITE, s * 2.4),
    starB = glow(st, starTex(T), '#ffffff', s * 5.5), halo = glow(st, spriteTex(T), STORM_BLUE, s * 3.4);
  hotA.position.copy(src); hotB.position.copy(hit); starB.position.copy(hit); halo.position.copy(p0);
  hotA.renderOrder = 61; hotB.renderOrder = 62; starB.renderOrder = 63; halo.renderOrder = 61;
  [hotA, hotB, starB].forEach(function (o) { o.material.depthTest = false; });
  st.scene.add(edim, hotA, hotB, starB, halo);
  if (e && e.from) { hotA.visible = true; } else hotA.visible = false;

  // Sparks thrown off the strike and the crown: short streaks that fall to the page.
  var NS = hd ? 22 : 12, sp = new Float32Array(NS * 8), sn = 0;
  function emit(x, y, z, vx, vy, vz, life) { var o = (sn++ % NS) * 8; sp[o] = x; sp[o + 1] = y; sp[o + 2] = z; sp[o + 3] = vx; sp[o + 4] = vy; sp[o + 5] = vz; sp[o + 6] = 0; sp[o + 7] = life; }
  function ringPt(out, a, rad, lift) { return out.copy(top).addScaledVector(U, Math.cos(a) * rad).addScaledVector(V, Math.sin(a) * rad).addScaledVector(nrm, lift); }
  var pt = new T.Vector3(), pe = new T.Vector3(), off = new T.Vector3(), jolt = 0, struck = false, gf = glowFace(die, fi);

  function strike() {
    struck = true; jolt = 0.16;
    for (var k = 0; k < (hd ? 12 : 7); k++) { var a = rnd() * 6.283, v = 2 + rnd() * 3; emit(hit.x, hit.y + s * 0.1, hit.z, Math.cos(a) * v, 1.5 + rnd() * 3.5, Math.sin(a) * v, 0.25 + rnd() * 0.3); }
  }
  return { update: function (dt) {
    t += dt; frame++;
    if (t >= END) {
      drop(st, [rb.mesh, edim, hotA, hotB, starB, halo]);
      g.position.copy(p0); g.quaternion.copy(q0); g.updateMatrixWorld(true);
      tintDie(die, th.burn, 0); glowNumeral(die, fi, NUM);
      return false;
    }
    var tau = t - bolt.t0, sd = Math.floor(frame / 2) * 7919 + 13;
    // The die shudders as the bolt lands, and hops a little.
    off.set(0, 0, 0);
    if (tau >= 0 && tau < 0.3) { var ja = s * 0.03 * Math.exp(-tau * 16); off.set((rnd() - 0.5) * ja, s * 0.07 * Math.sin(clamp01(tau / 0.22) * Math.PI) + rnd() * ja * 0.3, (rnd() - 0.5) * ja); }
    g.position.set(p0.x + off.x, p0.y + off.y, p0.z + off.z);

    rb.begin();
    var lv = stormBolt(rb, s, bolt, tau);
    if (!struck && tau >= 0) strike();
    var env = tau >= 0 ? Math.exp(-tau * 7) : 0, crown = tau >= 0 ? smooth(0, 0.1, tau) * (1 - smooth(0.55, 1.45, tau)) : 0, flick = 0.65 + 0.35 * stormRnd();

    if (crown > 0.02) {
      var base = crown * 0.9 * flick, i, k, n, P = rb.P, W = rb.W, I = rb.I;
      // Arcs round the rim, a few at a time, each a new stretch of it every other frame.
      var na = hd ? 3 : 2;
      for (k = 0; k < na; k++) {
        stormSeed(sd + k * 91); var a0 = stormRnd() * 6.283, span = 0.9 + stormRnd() * 1.3; n = 10;
        for (i = 0; i < n; i++) {
          var u = i / (n - 1); ringPt(pt, a0 + span * u, Rr * (1 + (stormRnd() - 0.5) * 0.3), s * (0.06 + (stormRnd() - 0.5) * 0.12)).add(off);
          P[i * 3] = pt.x; P[i * 3 + 1] = pt.y; P[i * 3 + 2] = pt.z; W[i] = s * 0.05 * (0.8 + 0.4 * stormRnd()); I[i] = base * 1.15 * (0.55 + 0.45 * stormRnd()) * Math.min(1, Math.sin(Math.PI * u) * 3 + 0.3);
        }
        rb.push(n, 0.24, 0.3);
      }
      // Spikes thrown outward and up from the ring.
      var nk = hd ? 8 : 5, dens = 0.4 + 0.6 * crown;
      for (k = 0; k < nk; k++) {
        stormSeed(sd + k * 131 + 7); if (stormRnd() > dens) continue;
        var ph = (k + stormRnd() * 0.7) / nk * 6.283 + t * 1.3, len = s * (0.8 + 0.7 * stormRnd()) * (0.5 + 0.5 * crown);
        ringPt(pt, ph, Rr * 0.95, s * 0.05).add(off);
        pe.copy(pt).addScaledVector(U, Math.cos(ph) * len * 0.85).addScaledVector(V, Math.sin(ph) * len * 0.85).addScaledVector(nrm, len * 0.7);
        n = stormChannel(P, 0, pt.x, pt.y, pt.z, pe.x, pe.y, pe.z, 3, 0.3, sd + k, sd + k + 1, 0);
        for (i = 0; i < n; i++) { var uu = i / (n - 1); W[i] = s * 0.05 * (1 - 0.5 * uu); I[i] = base * 1.2 * (0.9 - 0.55 * uu) * (0.6 + 0.4 * stormRnd()); }
        rb.push(n, 0.24, 0.5);
        if (stormRnd() < 0.2 * dens && !(frame & 1)) emit(pe.x, pe.y, pe.z, (pe.x - pt.x) * 4, 1 + stormRnd() * 2, (pe.z - pt.z) * 4, 0.2 + stormRnd() * 0.25);
      }
    }
    // The sparks in flight.
    for (var q = 0; q < NS; q++) {
      var o = q * 8; if (sp[o + 6] >= sp[o + 7]) continue;
      sp[o + 6] += dt; sp[o + 4] -= 10 * dt;
      sp[o] += sp[o + 3] * dt; sp[o + 1] += sp[o + 4] * dt; sp[o + 2] += sp[o + 5] * dt;
      if (sp[o + 1] < 0.04) { sp[o + 1] = 0.04; sp[o + 4] *= -0.25; sp[o + 3] *= 0.6; sp[o + 5] *= 0.6; }
      var kk = sp[o + 6] / sp[o + 7]; if (kk >= 1) continue;
      var Q = rb.P, vx = sp[o + 3], vy = sp[o + 4], vz = sp[o + 5], vl = Math.sqrt(vx * vx + vy * vy + vz * vz) || 1, tl = Math.min(0.06, vl * 0.03) / vl;
      Q[0] = sp[o]; Q[1] = sp[o + 1]; Q[2] = sp[o + 2]; Q[3] = sp[o] - vx * tl * 1.0; Q[4] = sp[o + 1] - vy * tl; Q[5] = sp[o + 2] - vz * tl;
      rb.W[0] = s * 0.022; rb.W[1] = s * 0.012; rb.I[0] = 0.95 * (1 - kk); rb.I[1] = 0.3 * (1 - kk);
      rb.push(2, 0.3, 0.2);
    }
    rb.end();

    // Flash on arrival, and the dark patch lifting with it (on a pale page that is the flash).
    hotA.material.opacity = e && e.from ? 0.7 * Math.max(0, 1 - Math.abs(tau + 0.05) / 0.12) : 0; hotA.visible = hotA.material.opacity > 0.003;
    hotB.material.opacity = Math.min(1, lv * 0.9); hotB.scale.setScalar(s * (1.0 + 1.3 * env)); hotB.visible = hotB.material.opacity > 0.003;
    starB.material.opacity = 0.65 * env; starB.scale.setScalar(s * 4.4 * (0.7 + 0.5 * (1 - env))); starB.material.rotation = tau * 0.6; starB.visible = starB.material.opacity > 0.003;
    halo.material.opacity = 0.1 * crown + 0.3 * lv * (tau >= 0 ? 1 : 0); halo.visible = halo.material.opacity > 0.003;
    edim.material.opacity = 0.42 * smooth(-0.1, 0.12, tau) * (1 - smooth(0.7, 1.4, tau)) * (1 - 0.5 * env); edim.visible = edim.material.opacity > 0.003;

    // The die: lit blue by the stroke, the 20 white-hot then settling to the storm's blue, crackling.
    tintDie(die, STORM_BLUE, lv * 1.8 + crown * 0.4 * flick);
    if (!(frame & 1) && tau >= -0.05) stormPaintFace(gf, mix(NUM, '#ffffff', env * 0.7), 1.7 + 3 * lv + 1.1 * crown * flick, Math.round(crown * (hd ? 3 : 2)), frame * 31 + 7);
    return true;
  } };
}
// beat: the last bolt from overhead; peak: the flash and the shockwave are spent and the cloud is thrown back.
registerCrit('storm', { name: 'Storm call', kind: 'succ', what: 'A storm gathers over the page and lightning strikes the die again and again, the last bolt from straight above, leaving the 20 crackling with light.', fx: fxStorm,
  beat: 2.85, peak: 3.7, echo: fxStormEcho, echoWhat: 'A bolt jumps from the grand die to this one and leaves a crown of sparks.' });
