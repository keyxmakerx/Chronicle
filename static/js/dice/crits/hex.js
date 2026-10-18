// ---------- Crit fail: Hex ----------
// A curse laid on the page. A burning line runs round the die and scorches a circle; a ring of runes is
// branded in stroke by stroke, white-hot at the pen and cooling to charred edges round an ember core,
// while an inner ring turns the other way and thin smoke curls up from the burnt lines, lit from below
// by the embers. The die is bathed red and pulses once as the hex takes; then the fire dies, the runes
// fade to faint scorch marks, the smoke drifts off, and the 1 is left burning red.
// The echo is one rune branded under another die, with a burning line run to it from the grand's ring.
import {
  MARKS, canvas, clamp01, drop, env4, fxLight, glowFace, mix, mulberry, numberOnto, out3, polyLen,
  registerCrit, shadeDie, smooth, spriteTex, starTex, strokeW, tintDie, share
} from '../engine.js';

var HEX_RED = '#ff3322', HEX_SOOT = '#2c2220', HEX_ASH = '#b3a29a';
var HEX_TAKE = 2.3, HEX_COOL = 2.85, HEX_DUR = 4.6;

// Noise and the heat ramp. Prefixed so it never collides with another effect's copy.
var HEX_GLSL = [
  'float hxH(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float hxN(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(hxH(i), hxH(i + vec2(1.0, 0.0)), u.x), mix(hxH(i + vec2(0.0, 1.0)), hxH(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float hxF(vec2 p){ return 0.55 * hxN(p) + 0.3 * hxN(p * 2.1 + 3.7) + 0.15 * hxN(p * 4.3 + 8.1); }',
  // Dying ember, red, orange, yellow, white-hot: the colours of metal cooling, in linear light.
  'vec3 hxRamp(float h){',
  ' vec3 c0 = vec3(0.22, 0.008, 0.003), c1 = vec3(0.85, 0.03, 0.01), c2 = vec3(1.0, 0.2, 0.02), c3 = vec3(1.0, 0.5, 0.1), c4 = vec3(1.0, 0.78, 0.4);',
  ' h = clamp(h, 0.0, 1.0); vec3 c = mix(c0, c1, smoothstep(0.0, 0.3, h)); c = mix(c, c2, smoothstep(0.3, 0.6, h)); c = mix(c, c3, smoothstep(0.6, 0.85, h));',
  ' return mix(c, c4, smoothstep(0.85, 1.0, h)); }'].join('\n');

// A pale stone needs the number in a deep red: red light on ivory has nothing to stand on.
function hexPale(th) {
  var h = th && th.base && th.base.grad && th.base.grad[0];
  if (!h || h.charAt(0) !== '#') return false;
  var n = parseInt(h.slice(1), 16);
  return (0.3 * (n >> 16 & 255) + 0.59 * (n >> 8 & 255) + 0.11 * (n & 255)) / 255 > 0.62;
}
function hexHash(a) { var x = Math.sin(a * 127.1 + 311.7) * 43758.5453; return x - Math.floor(x); }

// How far the rings have turned: a steady turn that eases to a near stop as the hex takes.
function hexSpin(t) {
  var a = 2.2, b = 3.5, x, I;
  if (t < a) I = 0; else if (t < b) { x = (t - a) / (b - a); I = (b - a) * (x * x * x - x * x * x * x / 2); } else I = (b - a) * 0.5 + (t - b);
  return t - 0.92 * I;
}

// A rune: one of the engine's rune marks, or two small ones side by side, mirrored now and then.
// Strokes in a box one unit tall, centred on the origin, y up. They spell nothing.
function hexGlyph(rnd) {
  var out = [], mk = MARKS[Math.floor(rnd() * MARKS.length)], flip = rnd() < 0.4 ? -1 : 1, mnx = 9, mxx = -9;
  function put(m, ox, oy, sx, sy) { m.forEach(function (pl) { out.push(pl.map(function (p) { return [ox + p[0] * sx, oy + p[1] * sy]; })); }); }
  if (rnd() < 0.32) put(mk, 0, 0.12, 0.62, 0.76), put(MARKS[Math.floor(rnd() * MARKS.length)], 0.34, 0.12, 0.62, 0.76);
  else put(mk, 0, 0, 1, 1);
  out.forEach(function (pl) { pl.forEach(function (p) { mnx = Math.min(mnx, p[0]); mxx = Math.max(mxx, p[0]); }); });
  var cx = (mnx + mxx) / 2;
  return out.map(function (pl) { return pl.map(function (p) { return [(p[0] - cx) * flip, 0.5 - p[1]]; }); });
}

// A stroke to be burnt in: points on the page, half-width of its ribbon, and when the pen starts and
// ends. The time at each point follows the distance along the stroke.
function hexStroke(pts, w, t0, t1, seed) {
  var cum = [0], L = 0, i;
  for (i = 1; i < pts.length; i++) { L += Math.hypot(pts[i][0] - pts[i - 1][0], pts[i][1] - pts[i - 1][1]); cum.push(L); }
  return { pts: pts, w: w, t0: t0, t1: t1, cum: cum, L: L, seed: seed, head: false };
}
var HEXPT = { x: 0, z: 0 };
// The point a fraction u of the way along a stroke, in HEXPT.
function hexAlong(sk, u) {
  var d = u * sk.L, i = 1, c = sk.cum, P = sk.pts;
  while (i < c.length - 1 && c[i] < d) i++;
  var f = c[i] > c[i - 1] ? (d - c[i - 1]) / (c[i] - c[i - 1]) : 0;
  HEXPT.x = P[i - 1][0] + (P[i][0] - P[i - 1][0]) * f; HEXPT.z = P[i - 1][1] + (P[i][1] - P[i - 1][1]) * f;
  return HEXPT;
}

// Strokes as flat ribbons on the page: mitred joins, round caps, and an ignition time at every vertex
// so a stroke can be burnt in point by point. aSide runs across the ribbon (-1..1); aEnd is how far into
// a cap a vertex is, so length(aSide, aEnd) is the true distance from the line, round at the ends.
function hexRibbonGeo(T, strokes) {
  var pos = [], aT = [], aS = [], aE = [], aR = [], idx = [], base = 0;
  strokes.forEach(function (sk) {
    var P = sk.pts, n = P.length, w = sk.w, d = [], rows = [], i, dt = sk.t1 - sk.t0;
    if (n < 2 || sk.L < 1e-6) return;
    for (i = 0; i < n - 1; i++) { var dx = P[i + 1][0] - P[i][0], dz = P[i + 1][1] - P[i][1], l = Math.hypot(dx, dz) || 1; d.push([dx / l, dz / l]); }
    function tAt(k) { return sk.t0 + dt * sk.cum[k] / sk.L; }
    rows.push([P[0][0] - d[0][0] * w, P[0][1] - d[0][1] * w, -d[0][1] * w, d[0][0] * w, tAt(0), 1]);
    for (i = 0; i < n; i++) {
      var nx, nz, sc = 1;
      if (i === 0) { nx = -d[0][1]; nz = d[0][0]; }
      else if (i === n - 1) { nx = -d[n - 2][1]; nz = d[n - 2][0]; }
      else {
        var ax = -d[i - 1][1], az = d[i - 1][0], bx = -d[i][1], bz = d[i][0], mx = ax + bx, mz = az + bz, ml = Math.hypot(mx, mz);
        if (ml < 1e-4) { nx = bx; nz = bz; } else { mx /= ml; mz /= ml; sc = Math.min(2.2, 1 / Math.max(0.3, mx * bx + mz * bz)); nx = mx; nz = mz; }
      }
      rows.push([P[i][0], P[i][1], nx * w * sc, nz * w * sc, tAt(i), 0]);
    }
    rows.push([P[n - 1][0] + d[n - 2][0] * w, P[n - 1][1] + d[n - 2][1] * w, -d[n - 2][1] * w, d[n - 2][0] * w, tAt(n - 1), 1]);
    rows.forEach(function (r, k) {
      pos.push(r[0] + r[2], 0, r[1] + r[3], r[0] - r[2], 0, r[1] - r[3]);
      aT.push(r[4], r[4]); aS.push(1, -1); aE.push(r[5], r[5]); aR.push(sk.seed, sk.seed);
      if (k) { var a = base + 2 * (k - 1), c = base + 2 * k; idx.push(a, a + 1, c, a + 1, c + 1, c); }
    });
    base += rows.length * 2;
  });
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('aT', new T.Float32BufferAttribute(aT, 1));
  geo.setAttribute('aSide', new T.Float32BufferAttribute(aS, 1)); geo.setAttribute('aEnd', new T.Float32BufferAttribute(aE, 1));
  geo.setAttribute('aSeed', new T.Float32BufferAttribute(aR, 1)); geo.setIndex(idx);
  return geo;
}

// The brand: a burnt line seen from above. It lights as the pen reaches it (white-hot, then red, then an
// ember that gutters), keeps a dark charred body with ragged edges and a wide brown scorch round it, and
// when the fire dies the ember goes out and the char fades. Light is added and the char darkens in the
// same pass (premultiplied), so it reads on a white page as well as a dark one.
function hexBrandMat(st, o) {
  var T = st.T, cl = [], i;
  for (i = 0; i < 6; i++) cl.push(new T.Vector3(1e4, 1e4, 1));
  var uni = { uNow: { value: 0 }, uPulse: { value: 0 }, uCool: { value: o.cool }, uCoolSp: { value: o.coolSp }, uCoolD: { value: o.coolD }, uFade: { value: 1 }, uAmp: { value: o.amp },
    uFlick: { value: 1 }, uLin: { value: st.cine ? 1.3 : 1 }, uNC: { value: 0 }, uClear: { value: cl }, uSwR: { value: 0 }, uSwW: { value: 1 }, uSw: { value: 0 } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false, premultipliedAlpha: true, side: T.DoubleSide,
    vertexShader: ['attribute float aT; attribute float aSide; attribute float aEnd; attribute float aSeed;',
      'varying float vT; varying float vSide; varying float vEnd; varying float vSeed; varying vec2 vL; varying vec2 vW;',
      'void main(){ vT = aT; vSide = aSide; vEnd = aEnd; vSeed = aSeed; vL = position.xz; vW = (modelMatrix * vec4(position, 1.0)).xz;',
      ' gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }'].join('\n'),
    fragmentShader: ['uniform float uNow, uPulse, uCool, uCoolSp, uCoolD, uFade, uAmp, uFlick, uLin, uNC, uSwR, uSwW, uSw; uniform vec3 uClear[6];',
      'varying float vT; varying float vSide; varying float vEnd; varying float vSeed; varying vec2 vL; varying vec2 vW;', HEX_GLSL,
      'void main(){',
      ' float age = uNow - vT; if (age < -0.03) discard;',
      ' float rr = length(vec2(vSide, vEnd)); if (rr > 1.0) discard;',
      ' float lit = smoothstep(-0.03, 0.02, age), a1 = max(age, 0.0);',
      ' float n1 = hxF(vL * 7.0 + vSeed * 41.0), n2 = hxN(vL * 23.0 + vSeed * 13.0);',
      ' float cool = smoothstep(uCool + vSeed * uCoolSp, uCool + vSeed * uCoolSp + uCoolD, uNow);',
      // White-hot at the pen, red within half a second, then an ember that gutters until the fire dies.
      ' float fresh = exp(-a1 / 0.2);',
      ' float flick = 0.5 + 0.5 * sin(uNow * 9.0 + vSeed * 80.0 + n1 * 11.0) * sin(uNow * 4.3 + vSeed * 17.0);',
      ' float hold = (0.2 + 0.22 * flick) * uFlick * (1.0 - cool);',
      // The take sweeps out through the rings from the die: each stroke flares as the front passes.
      ' float sw = uSw * exp(-pow((length(vL) - uSwR) / uSwW, 2.0));',
      ' float h = clamp(fresh * 0.95 + hold + uPulse * 0.15 + sw * 0.25, 0.0, 1.0);',
      ' float soak = smoothstep(0.0, 0.5, a1);',
      ' float edgeChar = 0.58 + (n1 - 0.5) * 0.28;',
      ' float edgeCore = (0.27 + (n2 - 0.5) * 0.14) * (1.0 + 0.45 * fresh + 0.3 * uPulse + 0.9 * sw);',
      ' edgeCore = min(edgeCore, 0.5);',
      ' float aChar = 1.0 - smoothstep(edgeChar - 0.12, edgeChar + 0.03, rr);',
      ' float blot = 0.55 + 0.9 * n1;',
      ' float aHalo = (1.0 - smoothstep(0.2, mix(0.5, 1.0, soak), rr)) * 0.34 * blot;',
      ' float a = min(max(aChar * 0.95, aHalo) * lit * uFade * uLin, 1.0);',
      ' vec3 dark = mix(vec3(0.13, 0.05, 0.025), vec3(0.02, 0.008, 0.004), aChar);',
      ' float cm = (1.0 - smoothstep(edgeCore - 0.10, edgeCore + 0.03, rr)) * lit * max(max(fresh, 1.0 - cool), max(uPulse, sw));',
      // Specks of ember dying in the char.
      ' float sp = step(0.9, hxN(vL * 70.0 + floor(uNow * 7.0) * 5.3 + vSeed * 9.0)) * (1.0 - cool) * smoothstep(0.35, 0.5, rr) * (1.0 - smoothstep(0.55, 0.62, rr)) * lit * uFlick;',
      ' vec3 E = hxRamp(h) * (0.9 + 1.7 * fresh + 0.2 * uPulse + 0.3 * sw) * uAmp + hxRamp(0.55) * sp * 0.9 * uAmp;',
      ' vec3 rgb = dark * a * (1.0 - cm) + E * cm;',
      ' float al = max(a, cm);',
      ' rgb += hxRamp(0.45 + 0.4 * fresh) * exp(-rr * 2.6) * (fresh * 0.9 + hold * 0.5 + uPulse * 0.1 + sw * 0.3) * lit * uAmp * 0.35;',
      // A hole round each die that must stay in view.
      ' float hole = 1.0;',
      ' for (int i = 0; i < 6; i++) { if (float(i) >= uNC) break; hole *= smoothstep(uClear[i].z * 0.85, uClear[i].z * 1.2, distance(vW, uClear[i].xy)); }',
      ' gl_FragColor = vec4(rgb * hole, al * hole);',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
  mat.userData.uni = uni;
  return mat;
}

// The dark patch under the spell, with a hole round every die that must stay in view.
function hexDimMat(st, R) {
  var T = st.T, cl = [], i;
  for (i = 0; i < 6; i++) cl.push(new T.Vector3(1e4, 1e4, 1));
  var uni = { uOp: { value: 0 }, uR: { value: R }, uNC: { value: 0 }, uClear: { value: cl } };
  var mat = new T.ShaderMaterial({ uniforms: uni, transparent: true, depthWrite: false, premultipliedAlpha: true,
    vertexShader: 'varying vec2 vP; varying vec2 vW; void main(){ vP = position.xy; vW = (modelMatrix * vec4(position, 1.0)).xz; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }',
    fragmentShader: ['uniform float uOp, uR, uNC; uniform vec3 uClear[6]; varying vec2 vP; varying vec2 vW;',
      'void main(){ float d = length(vP) / uR; float a = 1.0 - smoothstep(0.2, 1.0, d); a = a * a * uOp;',
      ' for (int i = 0; i < 6; i++) { if (float(i) >= uNC) break; a *= smoothstep(uClear[i].z * 0.8, uClear[i].z * 1.25, distance(vW, uClear[i].xy)); }',
      ' gl_FragColor = vec4(vec3(0.05, 0.012, 0.01) * a, a);',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
  mat.userData.uni = uni;
  return mat;
}
function hexSetClear(uni, clear) {
  var n = Math.min(6, clear.length);
  for (var i = 0; i < n; i++) uni.uClear.value[i].set(clear[i].x, clear[i].z, clear[i].r);
  uni.uNC.value = n;
}

// A soft cloudy puff for the smoke, built once from warped value noise.
var HEXSMOKE = null;
function hexSmokeTex(T) {
  if (HEXSMOKE) return HEXSMOKE;
  var N = 128, c = canvas(N), g = c.getContext('2d'), im = g.createImageData(N, N), rnd = mulberry(77), L = 32, lat = new Float32Array(L * L);
  for (var i = 0; i < L * L; i++) lat[i] = rnd();
  function vn(x, y) {
    var xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi; xf = xf * xf * (3 - 2 * xf); yf = yf * yf * (3 - 2 * yf);
    var a = lat[(yi & 31) * L + (xi & 31)], b = lat[(yi & 31) * L + (xi + 1 & 31)], c2 = lat[(yi + 1 & 31) * L + (xi & 31)], d = lat[(yi + 1 & 31) * L + (xi + 1 & 31)];
    return a + (b - a) * xf + (c2 - a) * yf + (a - b - c2 + d) * xf * yf;
  }
  for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
    var u = x / N, v = y / N, r = Math.hypot(u - 0.5, v - 0.5) * 2, f = 0, amp = 0.55, fr = 2, wu = u + 0.18 * (vn(u * 4 + 9, v * 4 + 2) - 0.5), wv = v + 0.18 * (vn(u * 4 + 5, v * 4 + 11) - 0.5);
    for (var o = 0; o < 3; o++) { f += amp * vn(wu * fr + o * 7.1, wv * fr + o * 3.3); fr *= 2; amp *= 0.5; }
    var dn = clamp01(Math.exp(-r * r * 2.6) * (0.35 + 1.5 * (f - 0.25))) * (1 - smooth(0.75, 1, r)), k = (y * N + x) * 4;
    im.data[k] = im.data[k + 1] = im.data[k + 2] = Math.round(dn * 255); im.data[k + 3] = 255;
  }
  g.putImageData(im, 0, 0);
  HEXSMOKE = new T.CanvasTexture(c); share(HEXSMOKE);
  return HEXSMOKE;
}

// Smoke: flat puffs that rise off the page (the camera looks straight down, so they grow as they climb
// and sway as they go). Each is lit from below by the embers: the side of a puff that faces its source
// glows red-orange, most of all while it is young, and the far side stays soot or pale ash.
// place() sets one puff; it fades any puff that drifts over a place that must stay clear.
function hexSmoke(st, n, holes) {
  var T = st.T, geo = new T.PlaneGeometry(1, 1), aA = new Float32Array(n), aT = new Float32Array(n), aH = new Float32Array(n), aD = new Float32Array(n * 2), i;
  geo.rotateX(-Math.PI / 2);
  geo.setAttribute('aAlpha', new T.InstancedBufferAttribute(aA, 1)); geo.setAttribute('aTone', new T.InstancedBufferAttribute(aT, 1));
  geo.setAttribute('aHeat', new T.InstancedBufferAttribute(aH, 1)); geo.setAttribute('aDir', new T.InstancedBufferAttribute(aD, 2));
  var mat = new T.ShaderMaterial({ transparent: true, depthWrite: false,
    uniforms: { uMap: { value: hexSmokeTex(T) }, uSoot: { value: new T.Color(HEX_SOOT) }, uAsh: { value: new T.Color(HEX_ASH) }, uEmber: { value: new T.Color('#ff5a1c') }, uBoost: { value: st.cine ? 1.5 : 1.25 } },
    vertexShader: ['attribute float aAlpha; attribute float aTone; attribute float aHeat; attribute vec2 aDir; varying vec2 vUv; varying float vA; varying float vT; varying float vH; varying vec2 vD;',
      'void main(){ vUv = uv; vA = aAlpha; vT = aTone; vH = aHeat; vD = aDir; gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0); }'].join('\n'),
    fragmentShader: ['uniform sampler2D uMap; uniform vec3 uSoot, uAsh, uEmber; uniform float uBoost; varying vec2 vUv; varying float vA; varying float vT; varying float vH; varying vec2 vD;',
      'void main(){',
      ' float tx = texture2D(uMap, vUv).r, d = tx * vA; if (d < 0.004) discard;',
      ' vec2 q = vUv - 0.5; float side = dot(normalize(q + vec2(1e-4)), vD) * 0.5 + 0.5;',
      ' float lit = vH * mix(0.22, 1.0, side * side) * (0.55 + 0.7 * tx);',
      ' vec3 col = mix(mix(uSoot, uAsh, vT), uEmber * (0.8 + 0.9 * tx), clamp(lit, 0.0, 1.0));',
      ' d = 1.0 - pow(1.0 - clamp(d * (1.0 + 0.5 * lit), 0.0, 1.0), mix(uBoost, 1.0, vT));',
      ' gl_FragColor = vec4(col, d);',
      ' #include <tonemapping_fragment>',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
  var mesh = new T.InstancedMesh(geo, mat, n), m4 = new T.Matrix4(), v3 = new T.Vector3(), ZERO = new T.Matrix4().makeScale(0, 0, 0);
  mesh.frustumCulled = false; mesh.renderOrder = 4; st.scene.add(mesh);
  for (i = 0; i < n; i++) mesh.setMatrixAt(i, ZERO);
  return { mesh: mesh, holes: holes,
    hide: function (k) { mesh.setMatrixAt(k, ZERO); aA[k] = 0; },
    // x0, z0: where it was born; ox, oz: the way it drifts out; age and life in seconds; r1, r2: the
    // randomness of the stream it belongs to (every puff of a stream follows the same winding path, so the
    // stream reads as one curl), jit: this puff's own; tone 0 soot to 1 ash; peak its strongest alpha; heat
    // how hot its source was; s the die's size; sk how big the puffs are.
    place: function (k, x0, z0, ox, oz, age, life, r1, r2, jit, tone, peak, heat, s, fade, sk) {
      var u = age / life, rise = 1 - Math.exp(-age), out = s * 0.9 * (1 - Math.exp(-age / 0.9)) * (0.6 + r1 * 0.8), grow = 1 - Math.exp(-age * 2.2);
      var tw = s * (0.62 * Math.sin(age * (2.0 + r2 * 1.4) + r1 * 6.28) + 0.2 * Math.sin(age * 5.6 + r2 * 6.28)) * grow;
      var x = x0 + ox * out - oz * tw + (jit - 0.5) * 0.12 * s * grow, z = z0 + oz * out + ox * tw + (jit - 0.5) * 0.1 * s * grow, y = 0.06 + s * 1.4 * rise, sz = s * sk * (0.1 + 0.5 * Math.pow(u, 0.8));
      var al = peak * fade * smooth(0, 0.18, u) * (1 - smooth(0.4, 1, u));
      for (var h = 0; h < holes.length; h++) { var H = holes[h]; al *= smooth(H.r * 0.8, H.r * 1.25 + sz * 0.4, Math.hypot(x - H.x, z - H.z)); }
      if (al < 0.004) { mesh.setMatrixAt(k, ZERO); aA[k] = 0; return; }
      var rot = jit * 6.28 + age * (jit - 0.5) * 0.8, cr = Math.cos(rot), sr = Math.sin(rot), lx = x0 - x, lz = z0 - z, ll = Math.hypot(lx, lz) || 1;
      m4.makeRotationY(rot); m4.scale(v3.set(sz * (1.15 + jit * 0.3), 1, sz)); m4.setPosition(x, y, z); mesh.setMatrixAt(k, m4);
      aA[k] = al; aT[k] = tone; aH[k] = heat * Math.exp(-age / 0.45);
      lx /= ll; lz /= ll; aD[k * 2] = lx * cr - lz * sr; aD[k * 2 + 1] = -(lx * sr + lz * cr);
    },
    commit: function () { mesh.instanceMatrix.needsUpdate = true; geo.attributes.aAlpha.needsUpdate = true; geo.attributes.aTone.needsUpdate = true; geo.attributes.aHeat.needsUpdate = true; geo.attributes.aDir.needsUpdate = true; } };
}

// A pool of points that sparks and embers are fired into, so many small ones cost one draw call.
function hexPool(st, n, o) {
  var T = st.T, pos = new Float32Array(n * 3), col = new Float32Array(n * 4), vel = new Float32Array(n * 3), age = new Float32Array(n), life = new Float32Array(n), cur = 0;
  var cs = o.colors.map(function (h) { return new T.Color(h).multiplyScalar(o.boost || 1); });
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('color', new T.BufferAttribute(col, 4));
  var pts = new T.Points(geo, new T.PointsMaterial({ size: st.ptSize(o.size), map: o.tex, transparent: true, depthWrite: false, vertexColors: true, sizeAttenuation: true,
    blending: o.add ? T.AdditiveBlending : T.NormalBlending, toneMapped: !o.add }));
  pts.frustumCulled = false; pts.renderOrder = o.order; st.scene.add(pts);
  return { obj: pts,
    emit: function (x, y, z, vx, vy, vz, lf, ci) {
      var i = cur, c = cs[ci % cs.length]; cur = (cur + 1) % n;
      pos[i * 3] = x; pos[i * 3 + 1] = y; pos[i * 3 + 2] = z; vel[i * 3] = vx; vel[i * 3 + 1] = vy; vel[i * 3 + 2] = vz;
      age[i] = 0; life[i] = lf; col[i * 4] = c.r; col[i * 4 + 1] = c.g; col[i * 4 + 2] = c.b; col[i * 4 + 3] = 0;
    },
    update: function (dt, fade) {
      var g = o.g || 0, dd = Math.max(0, 1 - (o.drag || 0) * dt);
      for (var i = 0; i < n; i++) {
        if (life[i] <= 0) continue;
        age[i] += dt;
        var k = age[i] / life[i], j = i * 3;
        if (k >= 1) { col[i * 4 + 3] = 0; life[i] = 0; continue; }
        vel[j + 1] -= g * dt; vel[j] *= dd; vel[j + 1] *= dd; vel[j + 2] *= dd;
        pos[j] += vel[j] * dt; pos[j + 1] += vel[j + 1] * dt; pos[j + 2] += vel[j + 2] * dt;
        if (pos[j + 1] < 0.03) { pos[j + 1] = 0.03; vel[j + 1] *= -0.3; vel[j] *= 0.7; vel[j + 2] *= 0.7; }
        var a = o.alpha * fade * smooth(0, 0.08, k) * (1 - smooth(o.hold || 0.4, 1, k));
        if (o.twinkle) a *= 0.35 + 0.65 * Math.pow(0.5 + 0.5 * Math.sin(age[i] * o.twinkle + i * 1.7), 2);
        col[i * 4 + 3] = a;
      }
      geo.attributes.position.needsUpdate = true; geo.attributes.color.needsUpdate = true;
    } };
}

// Where everything goes, in dice sizes from the middle: the line that runs round the die, an inner band
// of small runes that turns one way, and an outer band of big ones with a bezel of ticks that turns the
// other. Returns the strokes of each ring and the places smoke can rise from.
function hexLayout(s, hd, rnd) {
  var TAU = Math.PI * 2, a0 = Math.PI / 2, seg = hd ? 150 : 96, O = [], I = [], srcs = [];
  function arc(R, from, dir, n) { var p = []; for (var i = 0; i <= n; i++) { var a = from + dir * TAU * i / n; p.push([Math.cos(a) * R, Math.sin(a) * R]); } return p; }
  function add(list, pts, w, t0, t1, head) { var sk = hexStroke(pts, w, t0, t1, rnd()); sk.head = head; list.push(sk); return sk; }
  // A band of runes, each branded in a few strokes one after another, the band running round from the same
  // spot as the lines. Up on a rune points outward.
  function runes(list, plane, N, Rc, H, w, tStart, tSpan, dur) {
    for (var j = 0; j < N; j++) {
      var a = a0 + j / N * TAU, c = Math.cos(a), sn = Math.sin(a), cx = Rc * c, cz = Rc * sn, T0 = tStart + j / N * tSpan, tot = 0, run = 0, best = null, bl = 0;
      var pls = hexGlyph(rnd).map(function (pl) { return pl.map(function (p) { return [cx - sn * p[0] * H + c * p[1] * H, cz + c * p[0] * H + sn * p[1] * H]; }); });
      pls.forEach(function (pl) { tot += polyLen(pl); });
      pls.forEach(function (pl) {
        var l = polyLen(pl), sk = add(list, pl, w, T0 + dur * run / tot, T0 + dur * (run + l) / tot, true);
        run += l; if (l > bl) { bl = l; best = sk; }
      });
      var m = hexAlong(best, 0.5); srcs.push({ plane: plane, x: m.x, z: m.z, ready: T0 + dur + 0.05 });
    }
  }
  add(O, arc(1.25 * s, a0, 1, seg), 0.14 * s, 0.10, 0.95, true);
  add(O, arc(2.32 * s, a0 + Math.PI, 1, seg), 0.105 * s, 0.82, 1.42, true);
  add(O, arc(3.10 * s, a0 + Math.PI, -1, seg), 0.115 * s, 0.88, 1.52, true);
  add(I, arc(1.55 * s, a0, -1, seg), 0.07 * s, 1.0, 1.5, true);
  add(I, arc(2.07 * s, a0, 1, seg), 0.07 * s, 1.05, 1.55, true);
  runes(O, 0, hd ? 18 : 15, 2.71 * s, 0.56 * s, 0.095 * s, 1.0, 0.95, 0.26);
  runes(I, 1, hd ? 14 : 11, 1.81 * s, 0.32 * s, 0.07 * s, 1.2, 0.85, 0.22);
  var NT = hd ? 48 : 32;
  for (var k = 0; k < NT; k++) {
    var a = a0 + k / NT * TAU, c = Math.cos(a), sn = Math.sin(a), r1 = 3.2 * s, r2 = (k % 4 === 0 ? 3.46 : 3.34) * s, tk = 1.35 + k / NT * 0.75;
    add(O, [[c * r1, sn * r1], [c * r2, sn * r2]], 0.04 * s, tk, tk + 0.05, false);
  }
  srcs.sort(function (p, q) { return p.ready - q.ready; });
  return { O: O, I: I, srcs: srcs };
}

// The points of light at a pen: a soft red glow and a hot white-yellow core, one draw call each, moved
// by hand every frame.
function hexHeadSet(st, n, s, order) {
  var T = st.T, pos = new T.BufferAttribute(new Float32Array(n * 3), 3), arr = [];
  function mk(size, tex, r, g2, b) {
    var col = new Float32Array(n * 4), geo = new T.BufferGeometry();
    geo.setAttribute('position', pos); geo.setAttribute('color', new T.BufferAttribute(col, 4));
    var pts = new T.Points(geo, new T.PointsMaterial({ size: st.ptSize(size), map: tex, transparent: true, depthWrite: false, vertexColors: true, sizeAttenuation: true, toneMapped: true }));
    pts.frustumCulled = false; pts.renderOrder = order; st.scene.add(pts);
    return { pts: pts, col: col, r: r, g: g2, b: b };
  }
  arr.push(mk(s * 0.6, spriteTex(T), 1.0, 0.22, 0.04), mk(s * 0.36, starTex(T), 1.6, 1.2, 0.7));
  return { objs: arr.map(function (a) { return a.pts; }),
    set: function (k, x, y, z, a) {
      pos.array[k * 3] = x; pos.array[k * 3 + 1] = y; pos.array[k * 3 + 2] = z;
      for (var i = 0; i < 2; i++) { var h = arr[i], c = h.col; c[k * 4] = h.r; c[k * 4 + 1] = h.g; c[k * 4 + 2] = h.b; c[k * 4 + 3] = a * (i ? 1 : 0.55); }
    },
    off: function (from) { for (var k = from; k < n; k++) { arr[0].col[k * 4 + 3] = 0; arr[1].col[k * 4 + 3] = 0; } },
    commit: function () { pos.needsUpdate = true; arr[0].pts.geometry.attributes.color.needsUpdate = true; arr[1].pts.geometry.attributes.color.needsUpdate = true; } };
}

// The 1 burning in a colour. On a pale stone it is drawn as solid colour instead of white-cored light,
// which has nothing to stand on over ivory. Redrawn only when the colour changes.
function hexNumber(die, fi, pale) {
  var gf = glowFace(die, fi), fc = die.faces[fi], key = '';
  return { set: function (color, k) {
    if (color !== key) {
      key = color; gf.clear();
      if (pale && gf.lay.style !== 'classic') {
        var g2 = gf.g, w = strokeW(gf.lay);
        g2.lineJoin = 'round'; g2.lineCap = 'round'; g2.shadowColor = color; g2.shadowBlur = w; g2.strokeStyle = color; g2.lineWidth = w * 1.25;
        gf.lay.strokes.forEach(function (pl) { g2.beginPath(); g2.moveTo(pl[0][0], pl[0][1]); for (var q = 1; q < pl.length; q++) g2.lineTo(pl[q][0], pl[q][1]); g2.stroke(); });
        g2.shadowBlur = 0;
      } else numberOnto(gf.g, gf.lay, color);
      gf.commit();
    }
    fc.mat.emissiveIntensity = k;
  } };
}

// The take: swells fast, a short hold, then falls away.
function hexPulse(t) { return env4(t, 2.2, 2.34, 2.36, 2.95); }

function fxHex(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = st.q.detail !== 'basic', rnd = mulberry(6613), t = 0, i, j, k;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), sc0 = g.scale.clone(), at = { x: p0.x, z: p0.z };
  var clear = (o && o.clear || []).slice(0, 6), pale = hexPale(th), lay = hexLayout(s, hd, rnd), all = lay.O.concat(lay.I), nO = lay.O.length, own = [];
  var DIMR = 4.5 * s;

  // The dark patch under the spell, so the fire reads on a white page.
  var dimMat = hexDimMat(st, DIMR), dim = new T.Mesh(new T.PlaneGeometry(DIMR * 2, DIMR * 2), dimMat);
  dim.rotation.x = -Math.PI / 2; dim.position.set(at.x, 0.004, at.z); dim.renderOrder = -2; dim.frustumCulled = false; st.scene.add(dim); own.push(dim);
  hexSetClear(dimMat.userData.uni, clear);

  // The brand: two rings on the page, one material, so one compiled shader and a tiny geometry.
  var mat = hexBrandMat(st, { cool: HEX_COOL, coolSp: 0.7, coolD: 0.8, amp: 1 }), U = mat.userData.uni, gO = new T.Group(), gI = new T.Group();
  hexSetClear(U, clear);
  var mO = new T.Mesh(hexRibbonGeo(T, lay.O), mat), mI = new T.Mesh(hexRibbonGeo(T, lay.I), mat);
  mO.frustumCulled = mI.frustumCulled = false; mO.renderOrder = mI.renderOrder = 2; gO.add(mO); gI.add(mI);
  gO.position.set(at.x, 0.011, at.z); gI.position.set(at.x, 0.012, at.z); st.scene.add(gO, gI); own.push(gO, gI);

  // Smoke rising off the runes once they are burnt in, in streams: a puff every so often from the same
  // spot, each drifting out and swaying, so a stream curls like the smoke off a snuffed wick.
  var holes = clear.map(function (c) { return { x: c.x, z: c.z, r: c.r }; }); holes.push({ x: at.x, z: at.z, r: 0.95 * s });
  var NS = hd ? 14 : 8, NSP = hd ? 24 : 17, streams = [], sm = hexSmoke(st, NS * NSP, holes);
  for (i = 0; i < NS; i++) {
    var src = lay.srcs[Math.min(lay.srcs.length - 1, Math.floor((i + 0.5) / NS * lay.srcs.length))];
    streams.push({ pl: src.plane, x: src.x, z: src.z, t0: src.ready + rnd() * 0.15, dt: (hd ? 0.075 : 0.105) + rnd() * 0.01, tEnd: 3.3 + rnd() * 0.45, r1: rnd(), r2: rnd(), tone: i % 2 ? 1 : 0 });
  }
  own.push(sm.mesh);

  // Sparks off the pen, and embers lifting off the lines.
  var sparks = hexPool(st, hd ? 200 : 100, { size: s * 0.15, tex: starTex(T), add: false, colors: ['#ffd27a', '#ff8a2a', '#fff0c0'], boost: 1.3, g: s * 6, drag: 1.2, alpha: 1, order: 7, hold: 0.25 });
  var embers = hexPool(st, hd ? 140 : 70, { size: s * 0.11, tex: spriteTex(T), add: false, colors: ['#ff7a24', '#ff4a1a', '#ffb050'], g: -s * 0.5, drag: 0.5, alpha: 0.9, order: 6, hold: 0.5, twinkle: 17 });
  var heads = hexHeadSet(st, 24, s, 8);
  own.push(sparks.obj, embers.obj); heads.objs.forEach(function (x) { own.push(x); });
  var sparkRate = hd ? 70 : 35, emberRate = hd ? 40 : 20, sparkAcc = 0, emberAcc = 0, took = false;

  // One red light on the die in HD; Basic has none and gets its colour from the die's own glow.
  var lt = null;
  if (hd) {
    lt = fxLight(st, new T.Vector3(at.x, p0.y + s * 0.8, at.z), HEX_RED, HEX_DUR, function (tt) {
      var k = 7 * smooth(0.1, 1.0, tt) + 5 * smooth(1.0, 2.2, tt) + 38 * hexPulse(tt);
      return k * (1 - smooth(3.0, 4.2, tt));
    }, s * 7);
    st.fx.push(lt);
  }

  var num = hexNumber(die, fi, pale);

  function spawnSpark(x, z, n) {
    var sp = s / 1.25;
    for (var q = 0; q < n; q++) {
      var a = rnd() * 6.283, v = (0.6 + rnd() * 1.8) * sp;
      sparks.emit(x, 0.06, z, Math.cos(a) * v, (0.8 + rnd() * 2.2) * sp, Math.sin(a) * v, 0.35 + rnd() * 0.45, q);
    }
  }
  function spawnEmber(phiO, phiI, burst) {
    for (var tries = 0; tries < 6; tries++) {
      var sk = all[Math.floor(rnd() * all.length)];
      if (!sk.head || t < sk.t1 + 0.05) continue;
      var P = hexAlong(sk, rnd()), ph = all.indexOf(sk) < nO ? phiO : phiI, cc = Math.cos(ph), ss = Math.sin(ph), sp = s / 1.25;
      embers.emit(at.x + P.x * cc + P.z * ss, 0.07, at.z - P.x * ss + P.z * cc, (rnd() - 0.5) * 0.4 * sp, (0.5 + rnd() * 0.9) * sp * (burst ? 2 : 1), (rnd() - 0.5) * 0.4 * sp, 0.9 + rnd() * 0.9, Math.floor(rnd() * 3));
      return;
    }
  }

  return { update: function (dt) {
    t += dt;
    var spin = hexSpin(t), phiO = -0.17 * spin, phiI = 0.58 * spin, cO = Math.cos(phiO), sO = Math.sin(phiO), cI = Math.cos(phiI), sI = Math.sin(phiI), pulse = hexPulse(t);
    gO.rotation.y = phiO; gI.rotation.y = phiI;
    U.uNow.value = t; U.uPulse.value = pulse;
    // The scorch dims as the fire dies, and is gone by the end.
    U.uFade.value = (1 - 0.62 * smooth(3.3, 4.15, t)) * (1 - smooth(4.2, 4.58, t));
    dimMat.userData.uni.uOp.value = ((st.cine ? 0.5 : 0.46) * smooth(0.1, 1.0, t) + 0.3 * pulse) * (1 - smooth(3.5, 4.5, t));

    // The take: a flare runs out through the rings from the die, and the die swells with it.
    var swk = clamp01((t - 2.25) / 0.55);
    U.uSwR.value = s * (0.8 + 3.4 * out3(swk)); U.uSw.value = t < 2.25 ? 0 : 1 - smooth(0.55, 1, swk);
    if (!took && t >= HEX_TAKE) {
      took = true;
      for (j = 0; j < (hd ? 36 : 18); j++) spawnEmber(phiO, phiI, true);
    }
    var swell = 0.045 * pulse; g.scale.setScalar(1 + swell); g.position.y = p0.y + swell * p0.y;

    // The pens: a glow and a hot core where each stroke is being drawn, and sparks off them.
    var nh = 0;
    for (i = 0; i < all.length && nh < 24; i++) {
      var sk = all[i];
      if (!sk.head || t < sk.t0 || t > sk.t1 + 0.04) continue;
      var u = clamp01((t - sk.t0) / (sk.t1 - sk.t0)), P = hexAlong(sk, u), cc = i < nO ? cO : cI, ss = i < nO ? sO : sI;
      var wx = at.x + P.x * cc + P.z * ss, wz = at.z - P.x * ss + P.z * cc;
      heads.set(nh++, wx, 0.07, wz, 1 - smooth(0.92, 1.04, (t - sk.t0) / (sk.t1 - sk.t0)));
      sparkAcc += sparkRate * dt / Math.max(1, nh * 0.5);
      if (sparkAcc >= 1) { var ns = Math.floor(sparkAcc); sparkAcc -= ns; spawnSpark(wx, wz, ns); }
    }
    heads.off(nh); heads.commit();
    if (t > 0.9 && t < 3.3) { emberAcc += emberRate * dt; while (emberAcc >= 1) { emberAcc -= 1; spawnEmber(phiO, phiI); } }
    var pf = 1 - smooth(3.7, 4.3, t); sparks.update(dt, pf); embers.update(dt, pf);

    // Smoke.
    var sfade = (1 - smooth(3.9, 4.5, t)) * (1 + 0.3 * pulse);
    for (i = 0; i < NS; i++) {
      var S = streams[i], cyc = NSP * S.dt;
      for (j = 0; j < NSP; j++) {
        var idx = i * NSP + j, rel = t - S.t0 - j * S.dt;
        if (rel < 0) { sm.hide(idx); continue; }
        var mm = Math.floor(rel / cyc), age = rel - mm * cyc, birth = t - age;
        if (birth > S.tEnd) { sm.hide(idx); continue; }
        var ph = S.pl ? 0.58 * hexSpin(birth) : -0.17 * hexSpin(birth), c2 = Math.cos(ph), s2 = Math.sin(ph);
        var sx = at.x + S.x * c2 + S.z * s2, sz = at.z - S.x * s2 + S.z * c2, ox = sx - at.x, oz = sz - at.z, ol = Math.hypot(ox, oz) || 1;
        var jt = hexHash(i * 131 + j * 17 + mm * 3.1);
        sm.place(idx, sx, sz, ox / ol, oz / ol, age, cyc * 0.97, S.r1, S.r2, jt, S.tone, S.tone ? 0.45 : 0.6, (1 - smooth(2.9, 3.8, birth)) * (1 + 0.8 * pulse), s, sfade, 1);
      }
    }
    sm.commit();

    // The 1: it gutters red, then burns; it flares with the take and settles to a steady red.
    var nc, nk;
    if (t < 0.5) { nc = mix(th.burn, HEX_RED, Math.round(smooth(0, 0.5, t) * 10) / 10); nk = 1.3 + 0.7 * smooth(0, 0.5, t) + 0.3 * Math.sin(t * 41) * smooth(0.1, 0.4, t); }
    else { nc = HEX_RED; nk = (2.0 - 0.3 * smooth(3.0, 4.0, t)) + 0.45 * Math.sin(t * 9) * Math.sin(t * 3.7 + 1) * (1 - smooth(2.9, 3.6, t)) + 1.8 * pulse; }
    num.set(nc, nk);
    if (pale) shadeDie(die, 1 - 0.12 * smooth(0, 0.5, t));
    // Glass takes the red through its whole body, so it gets less, or the 1 inside it washes out.
    tintDie(die, HEX_RED, ((0.05 * smooth(0, 0.5, t) + 0.17 * smooth(0.9, 2.2, t)) + 0.34 * pulse) * (die.see === 'glass' ? 0.5 : 1) * (1 - smooth(3.0, 4.3, t)));

    if (t >= HEX_DUR) {
      sm.mesh.dispose(); drop(st, own);
      g.position.copy(p0); g.quaternion.copy(q0); g.scale.copy(sc0); tintDie(die, th.burn, 0);
      num.set(HEX_RED, pale ? 2 : 1.7);
      if (pale) shadeDie(die, 0.88);
      return false;
    }
    return true;
  } };
}

// The echo: a thin line of fire runs across the page from the grand's outer ring to this die, and one
// rune is branded under it, flickers, and smokes out. The rune is drawn big enough to show round the die.
function fxHexEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = st.q.detail !== 'basic', ord = e && e.order || 0, rnd = mulberry(404 + ord * 17), t = 0, i, own = [];
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), at = { x: p0.x, z: p0.z }, from = e && e.from ? e.from.group.position : null, pale = hexPale(th);
  var gx = 0, gz = 0, dist = 0, runT0 = 0.06, lineMat = null, lineMesh = null, line = null;
  // The fuse, if there is a grand die: it starts on the grand's outer ring, wavering a little as it runs.
  if (from) {
    gx = at.x - from.x; gz = at.z - from.z; dist = Math.hypot(gx, gz) || 1; gx /= dist; gz /= dist;
    var Rg = (e.from.size || s) * 3.3, sd = dist > Rg * 1.15 ? Rg : dist * 0.4, n = 36, pts = [], amp = s * 0.14, ph = rnd() * 6.28, len = dist - sd, lt0 = 0.04, ld = Math.min(0.55, Math.max(0.32, len / 14));
    for (i = 0; i <= n; i++) {
      var u = i / n, wob = amp * Math.sin(u * Math.PI * 3 + ph) * Math.sin(u * Math.PI), d = sd + len * u;
      pts.push([from.x + gx * d - gz * wob, from.z + gz * d + gx * wob]);
    }
    line = hexStroke(pts, 0.07 * s, lt0, lt0 + ld, rnd()); line.head = true; runT0 = lt0 + ld + 0.02;
    lineMat = hexBrandMat(st, { cool: 0.75, coolSp: 0.25, coolD: 0.45, amp: 0.8 });
    lineMesh = new T.Mesh(hexRibbonGeo(T, [line]), lineMat); lineMesh.frustumCulled = false; lineMesh.renderOrder = 60;
    lineMesh.position.set(0, 0.013, 0); st.scene.add(lineMesh); own.push(lineMesh);
  }
  var DUR = from ? 1.85 : 1.6, rD = 0.3;

  // The rune: an upright mark turned a random way and slid along its own axis, so its head and branches
  // show round the die's outline while its stem runs under the die (which hides the middle).
  var pick = [0, 6, 9, 1, 2], mk = MARKS[pick[(ord + Math.floor(rnd() * 3)) % pick.length]], H = 2.7 * s, ang = rnd() * Math.PI, ca = Math.cos(ang), sa = Math.sin(ang), flip = rnd() < 0.5 ? -1 : 1;
  var cx0 = at.x - sa * 0.6 * s, cz0 = at.z + ca * 0.6 * s;
  var pls = mk.map(function (pl) { return pl.map(function (p) { var u = (p[0] - 0.25) * H * flip, v = (0.5 - p[1]) * H; return [cx0 + u * ca - v * sa, cz0 + u * sa + v * ca]; }); });
  var tot = 0, run = 0, rs = [];
  pls.forEach(function (pl) { tot += polyLen(pl); });
  pls.forEach(function (pl) { var l = polyLen(pl), sk = hexStroke(pl, 0.13 * s, runT0 + rD * run / tot, runT0 + rD * (run + l) / tot, rnd()); sk.head = true; run += l; rs.push(sk); });
  var runMat = hexBrandMat(st, { cool: runT0 + rD + 0.5, coolSp: 0.3, coolD: 0.5, amp: 0.85 }), RU = runMat.userData.uni;
  var runMesh = new T.Mesh(hexRibbonGeo(T, rs), runMat); runMesh.frustumCulled = false; runMesh.renderOrder = 61; runMesh.position.set(0, 0.014, 0); st.scene.add(runMesh); own.push(runMesh);

  // Smoke off the parts of the rune that lie outside the die, never over its number.
  var NP = hd ? 9 : 5, sm = hexSmoke(st, NP, [{ x: at.x, z: at.z, r: 0.95 * s }]), cand = [], puffs = [];
  sm.mesh.renderOrder = 62; own.push(sm.mesh);
  rs.forEach(function (sk) { sk.pts.forEach(function (p) { if (Math.hypot(p[0] - at.x, p[1] - at.z) > 1.0 * s) cand.push(p); }); });
  if (!cand.length) cand.push([at.x + s * 1.3, at.z]);
  for (i = 0; i < NP; i++) {
    var c = cand[Math.floor(rnd() * cand.length)], ox = c[0] - at.x, oz = c[1] - at.z, ol = Math.hypot(ox, oz) || 1;
    puffs.push({ x: c[0], z: c[1], ox: ox / ol, oz: oz / ol, b: runT0 + rD + 0.05 + i / NP * 0.7 + rnd() * 0.1, r1: rnd(), r2: rnd(), tone: i % 2 });
  }
  var sparks = hexPool(st, hd ? 60 : 30, { size: s * 0.17, tex: starTex(T), add: false, colors: ['#ffd27a', '#ff8a2a', '#fff0c0'], boost: 1.2, g: s * 6, drag: 1.2, alpha: 0.9, order: 63, hold: 0.25 });
  own.push(sparks.obj);
  var heads = hexHeadSet(st, 4, s * 0.8, 64); heads.objs.forEach(function (x) { own.push(x); });
  var num = hexNumber(die, fi, pale), sparkAcc = 0, pens = line ? [line].concat(rs) : rs;

  return { update: function (dt) {
    t += dt;
    // The ember gutters like a candle while it lasts.
    var fl = 0.3 + 0.7 * hexHash(Math.floor(t * 17) + 3.3), live = smooth(runT0, runT0 + rD, t) * (1 - smooth(runT0 + rD + 0.65, runT0 + rD + 1.0, t));
    if (lineMat) { var LU = lineMat.userData.uni; LU.uNow.value = t; LU.uFade.value = 1 - smooth(1.0, 1.5, t); }
    RU.uNow.value = t; RU.uFlick.value = 0.35 + 0.65 * fl; RU.uFade.value = 1 - smooth(DUR - 0.6, DUR - 0.05, t);

    // Pens.
    var nh = 0;
    for (i = 0; i < pens.length && nh < 4; i++) {
      var sk = pens[i];
      if (t < sk.t0 || t > sk.t1 + 0.04) continue;
      var u2 = (t - sk.t0) / (sk.t1 - sk.t0), P = hexAlong(sk, clamp01(u2));
      heads.set(nh++, P.x, 0.07, P.z, 1 - smooth(0.92, 1.04, u2));
      sparkAcc += 50 * dt;
      if (sparkAcc >= 1) {
        var ns = Math.floor(sparkAcc), spd = s / 1.25; sparkAcc -= ns;
        for (var q = 0; q < ns; q++) { var a = rnd() * 6.283, v = (0.5 + rnd() * 1.4) * spd; sparks.emit(P.x, 0.06, P.z, Math.cos(a) * v, (0.6 + rnd() * 1.8) * spd, Math.sin(a) * v, 0.3 + rnd() * 0.35, q); }
      }
    }
    heads.off(nh); heads.commit(); sparks.update(dt, 1 - smooth(DUR - 0.5, DUR - 0.1, t));

    // Smoke.
    for (i = 0; i < NP; i++) {
      var pf = puffs[i], age = t - pf.b;
      if (age < 0 || age > 1.2) { sm.hide(i); continue; }
      sm.place(i, pf.x, pf.z, pf.ox, pf.oz, age, 1.2, pf.r1, pf.r2, pf.r2 * 0.7 + 0.15, pf.tone, pf.tone ? 0.34 : 0.5, 0.9 * (1 - smooth(runT0 + rD + 0.5, runT0 + rD + 1.1, pf.b)), s, 1, 1.7);
    }
    sm.commit();

    // The 1 flickers with the ember, then settles to a quiet red.
    if (t >= runT0) {
      var k1 = 1.1 + 1.5 * fl * live + 1.3 * (1 - smooth(runT0, runT0 + 0.12, t)) * smooth(runT0 - 0.02, runT0, t), settle = smooth(DUR - 0.7, DUR - 0.2, t);
      num.set(HEX_RED, hexMix2(k1, pale ? 1.6 : 1.3, settle));
      tintDie(die, HEX_RED, 0.2 * fl * live * (1 - settle) * (die.see === 'glass' ? 0.5 : 1));
      if (pale) shadeDie(die, 1 - 0.1 * smooth(runT0, runT0 + 0.2, t));
    }

    if (t >= DUR) {
      sm.mesh.dispose(); drop(st, own);
      g.position.copy(p0); g.quaternion.copy(q0); tintDie(die, th.burn, 0);
      num.set(HEX_RED, pale ? 1.6 : 1.3);
      if (pale) shadeDie(die, 0.9);
      return false;
    }
    return true;
  } };
}
function hexMix2(a, b, k) { return a + (b - a) * k; }
registerCrit('hex', { name: 'Hex', kind: 'fail', what: 'A line of fire runs round the die and a ring of red runes burns into the page, smoking, while an inner ring turns the other way. The die flares red as the hex takes, then the fire dies to faint scorch marks and the 1 is left burning red.',
  fx: fxHex, beat: HEX_TAKE, peak: 3.0, echo: fxHexEcho, echoWhat: 'A thin line of fire runs across the page to this die and one rune is branded under it, flickers and smokes out.' });
