// ---------- Crit success: Starfall ----------
// Night falls over the page round the 20: a soft-edged patch of sky, faint stars and a hint of nebula
// in it, the other dice left in clear air. Shooting stars streak in from its edge, one after another,
// each with a glowing trail, and land in bursts of light round the die. Every star that lands stays as
// a bright point, and fine lines of light draw themselves from star to star until the stars have built
// a crown round the die, the highest jewel last. The crown flares, a ring of silver runs out through
// it, the die is lit silver-white, then the sky fades and the crown goes out, leaving the 20 shining.
//
// The comets, the constellation's lines and the echo's trails are one ribbon mesh; the stars, landing
// rings and flashes are one instanced mesh of flat quads (the camera never moves and looks straight
// down, so a flat quad is already a billboard); sparks are one Points. The whole grand is five draws.
// Ribbons blend premultiplied, so a comet's dark halo still reads on a white page where light alone
// would vanish; the stars and flashes add light and live over the dark sky.
import {
  TAN, clamp01, drop, env4, glowFace, glowLayer, glowNumeral, isLight, mulberry, numberOnto, out3,
  registerCrit, shadeDie, smooth, tintDie, worldOf
} from '../engine.js';

var STARFALL = { white: '#f6f9ff', silver: '#dbe6ff', ice: '#9ccbff', blue: '#6e94ff', gold: '#ffe3a6', deep: '#02040c', indigo: '#0a1238',
  violet: '#6a3fc0', teal: '#1f7fb0', rose: '#b84a8a', dark: '#0d1450', num: '#eaf2ff', numDark: '#3a78ff', tint: '#dce8ff' };

// The crown, in units of its own radius with the die at the origin and page up at -z: x, z, weight (the
// star's size) and hue (0 ice, 1 gold). Peaks are the jewels. The valleys stay clear of the die's top
// edge, which sits about 0.35 out from the middle.
var STARFALL_CROWN = {
  p: [[-1.0, 0.9, 0.9, 0], [-1.0, 0.52, 0.9, 0], [-1.1, -0.95, 1.5, 0.7], [-0.82, -0.42, 0.85, 0], [-0.56, -1.08, 1.3, 0.6], [-0.28, -0.55, 0.85, 0], [0, -1.38, 2.1, 1],
    [0.28, -0.55, 0.85, 0], [0.56, -1.08, 1.3, 0.6], [0.82, -0.42, 0.85, 0], [1.1, -0.95, 1.5, 0.7], [1.0, 0.52, 0.9, 0], [1.0, 0.9, 0.9, 0], [-0.5, 0.71, 1.1, 0.4], [0, 0.71, 1.25, 0.5], [0.5, 0.71, 1.1, 0.4]],
  // Third number 1: a dimmer line (the string of band jewels).
  l: [[0, 1], [1, 2], [2, 3], [3, 4], [4, 5], [5, 6], [6, 7], [7, 8], [8, 9], [9, 10], [10, 11], [11, 12], [12, 0], [1, 11], [13, 14, 1], [14, 15, 1]],
  // Landing order: the band from both ends, its jewels, then the spikes in pairs, the highest jewel last.
  order: [0, 12, 1, 11, 13, 15, 14, 2, 10, 3, 9, 4, 8, 5, 7, 6]
};
// Where the crown sits round a die of size s, at radius R: the star positions on the page.
function starfallCrownAt(x, z, R, out) {
  var p = STARFALL_CROWN.p;
  for (var i = 0; i < p.length; i++) out.push({ x: x + p[i][0] * R, z: z + p[i][1] * R, w: p[i][2], h: p[i][3] });
  return out;
}
// The radius that keeps the whole crown on the canvas round a die at (x, z), never so small it meets the die.
function starfallFit(st, x, z, s) {
  var m = s * 0.4, R = s * 2.55;
  R = Math.min(R, (z + st.hh - m) / 1.4, (st.hh - z - m) / 0.94, (st.hw - Math.abs(x) - m) / 1.14);
  return Math.max(s * 1.9, R);
}

function starfallBack(x) { x = clamp01(x); var c1 = 1.7, c3 = c1 + 1; return 1 + c3 * Math.pow(x - 1, 3) + c1 * Math.pow(x - 1, 2); }
// How far the segment a to b passes from the point c (all on the page).
function starfallSegDist(ax, az, bx, bz, cx, cz) {
  var dx = bx - ax, dz = bz - az, l2 = dx * dx + dz * dz, u = l2 > 1e-9 ? clamp01(((cx - ax) * dx + (cz - az) * dz) / l2) : 0;
  return Math.hypot(ax + dx * u - cx, az + dz * u - cz);
}

// ---------- The sky ----------
// One sheet on the page: a ragged-edged patch of deep blue-black, nebula colour drifting in it and two
// layers of stars that twinkle. The edge is noisy so it reads as sky, not a disc. Round each place the
// other dice sit the sky thins to almost nothing, and the stars and nebula go with it.
var STARFALL_SKY_VS = 'uniform vec2 uC; varying vec2 vP; void main(){ vec4 w = modelMatrix * vec4(position, 1.0); vP = w.xz - uC; gl_Position = projectionMatrix * viewMatrix * w; }';
var STARFALL_SKY_FS = [
  'uniform float uTime, uR, uOp, uFlash; uniform vec3 uHole[12]; uniform vec3 uDeep, uIndigo, uViolet, uTeal, uRose, uSilver; varying vec2 vP;',
  'float sh(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'vec2 sh2(vec2 p){ return vec2(sh(p), sh(p + 19.19)); }',
  'float sn(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(sh(i), sh(i + vec2(1.0, 0.0)), u.x), mix(sh(i + vec2(0.0, 1.0)), sh(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float sf(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < DETAIL; i++) { s += a * sn(p); p = p * 2.03 + 1.7; a *= 0.5; } return s; }',
  // One layer of stars: a point in some cells, a small gaussian each, twinkling at its own pace; a few are warm.
  'vec3 stars(vec2 p, float cell, float dens, float rad, float bright, float seed){',
  ' vec2 g = p / cell, i = floor(g), f = fract(g); vec3 acc = vec3(0.0);',
  ' for (int y = -1; y <= 1; y++) for (int x = -1; x <= 1; x++) {',
  '  vec2 c = i + vec2(float(x), float(y)) + seed;',
  '  if (sh(c) > dens) continue;',
  '  vec2 d = (f - vec2(float(x), float(y)) - (0.12 + 0.76 * sh2(c))) * cell;',
  '  float b = 0.35 + 0.65 * sh(c + 3.7);',
  '  float tw = 0.62 + 0.38 * sin(uTime * (1.2 + 3.2 * sh(c + 8.1)) + 6.2832 * sh(c + 1.3));',
  '  float s = exp(-dot(d, d) / (rad * rad * (0.4 + b)) * 2.2) * b * tw * bright;',
  '  acc += mix(vec3(0.72, 0.85, 1.0), vec3(1.0, 0.9, 0.74), step(0.84, sh(c + 5.5))) * s;',
  ' }',
  ' return acc; }',
  'void main(){',
  ' float r = length(vP);',
  ' if (uOp <= 0.0 || r > uR * 1.45) discard;',
  ' float en = sf(vP * 0.2 + 4.0);',
  ' float edge = 1.0 - smoothstep(uR * 0.58, uR * 0.98, r + (en - 0.45) * uR * 0.36);',
  ' if (edge < 0.003) discard;',
  ' float hole = 0.0;',
  ' for (int i = 0; i < 12; i++) { vec3 h = uHole[i]; if (h.z > 0.0) hole = max(hole, 1.0 - smoothstep(h.z * 0.72, h.z * 1.25, length(vP - h.xy))); }',
  ' float keep = 1.0 - 0.88 * hole;',
  ' float a = uOp * edge * 0.95 * keep, vis = a;',
  ' vec2 q = vP * 0.1;',
  ' float n1 = sf(q + vec2(3.1 + uTime * 0.01, 1.7)), n2 = sf(q * 1.35 + vec2(11.3, 5.9 - uTime * 0.008)), n3 = sf(q * 0.8 + vec2(21.0, 9.0));',
  ' vec3 night = mix(uDeep, uIndigo, clamp(r / uR * 0.85 + (n1 - 0.5) * 0.3, 0.0, 1.0));',
  ' vec3 neb = uViolet * smoothstep(0.42, 0.78, n1) * 0.5 + uTeal * smoothstep(0.5, 0.82, n2) * 0.4 + uRose * smoothstep(0.55, 0.85, n3) * 0.26;',
  ' vec3 sk = stars(vP, 0.5, 0.5, 0.05, 0.9, 0.0);',
  ' #ifndef BASIC',
  ' sk += stars(vP, 1.6, 0.45, 0.085, 1.5, 7.0);',
  ' #endif',
  ' vec3 col = night * a + (neb * 0.34 + sk) * vis;',
  ' col += uSilver * uFlash * exp(-r * r / (uR * uR * 0.06)) * 0.45 * vis;',
  ' gl_FragColor = vec4(col, a);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function starfallSky(st, hd, C, R, clear) {
  var T = st.T, holes = [], i;
  for (i = 0; i < 12; i++) holes.push(i < clear.length ? new T.Vector3(clear[i].x - C.x, clear[i].z - C.z, clear[i].r) : new T.Vector3(0, 0, 0));
  var uni = { uC: { value: new T.Vector2(C.x, C.z) }, uTime: { value: 0 }, uR: { value: R }, uOp: { value: 0 }, uFlash: { value: 0 }, uHole: { value: holes },
    uDeep: { value: new T.Color(STARFALL.deep) }, uIndigo: { value: new T.Color(STARFALL.indigo) }, uViolet: { value: new T.Color(STARFALL.violet) },
    uTeal: { value: new T.Color(STARFALL.teal) }, uRose: { value: new T.Color(STARFALL.rose) }, uSilver: { value: new T.Color(STARFALL.silver) } };
  var mat = new T.ShaderMaterial({ uniforms: uni, vertexShader: STARFALL_SKY_VS, fragmentShader: STARFALL_SKY_FS, defines: hd ? { DETAIL: 4 } : { DETAIL: 3, BASIC: 1 }, transparent: true, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor });
  var m = new T.Mesh(new T.PlaneGeometry(R * 3, R * 3), mat);
  // Under the contact shadows and the die's own shadow, so they still darken the sky.
  m.rotation.x = -Math.PI / 2; m.position.set(C.x, 0.006, C.z); m.renderOrder = -3; m.frustumCulled = false;
  return { mesh: m, uni: uni };
}

// ---------- Ribbons: comet trails and constellation lines ----------
// aV is (distance along in half-widths, across -1..1); aI is (intensity, core share, hue 0 ice to 1
// gold, line length in half-widths or -1 for a comet). A line is a capsule with a shimmer running
// along it; a comet is a tapered ribbon with no caps. The core adds white-hot light; the halo adds its
// colour and also partly covers the page in a deep blue, which is what reads on white.
var STARFALL_RIB_VS = [
  'attribute vec2 aV; attribute vec4 aI; varying vec2 vV; varying vec4 vI;',
  'void main(){ vV = aV; vI = aI; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }'].join('\n');
var STARFALL_RIB_FS = [
  'uniform float uTime, uCover, uGlow; uniform vec3 uCore, uIce, uGold, uDark; varying vec2 vV; varying vec4 vI;',
  'void main(){',
  ' float I = vI.x, cf = vI.y, hue = vI.z, Lw = vI.w;',
  ' float dx = Lw > 0.0 ? max(0.0, max(-vV.x, vV.x - Lw)) : 0.0;',
  ' float d = sqrt(dx * dx + vV.y * vV.y);',
  ' if (d >= 1.0 || I <= 0.001) discard;',
  ' float shim = Lw > 0.0 ? 0.8 + 0.2 * sin(vV.x * 0.55 - uTime * 7.0) : 1.0;',
  ' float core = 1.0 - smoothstep(cf * 0.4, cf, d);',
  ' float inner = exp(-d * d / (cf * cf * 5.0));',
  ' float halo = pow(1.0 - d, 2.6);',
  ' vec3 hc = mix(uIce, uGold, clamp(hue, 0.0, 1.0));',
  ' vec3 col = (uCore * core * 2.2 + hc * inner * 1.2 * uGlow + hc * halo * 0.2 * uGlow) * I * shim;',
  ' float al = clamp((halo * 0.5 + inner * 0.4) * I * (Lw > 0.0 ? 0.44 : 1.25) * uCover, 0.0, 0.88);',
  ' gl_FragColor = vec4(col + uDark * al, al);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
// One mesh, rebuilt each frame from whatever is on: begin(), then fill P (xyz), W (half-width), I
// (intensity) and A (the along value) and push(...) once per ribbon, then end(). Ribbons lie flat on
// the page's plane because the camera looks straight down, so a side is just the page-plane normal.
function starfallRibbons(st, NV, order, cover, glow) {
  var T = st.T, pos = new Float32Array(NV * 3), av = new Float32Array(NV * 2), ai = new Float32Array(NV * 4), ix = new Uint16Array(NV * 3);
  var geo = new T.BufferGeometry(), pa = new T.BufferAttribute(pos, 3), va = new T.BufferAttribute(av, 2), ia = new T.BufferAttribute(ai, 4), xa = new T.BufferAttribute(ix, 1);
  [pa, va, ia, xa].forEach(function (b) { b.setUsage(T.DynamicDrawUsage); });
  geo.setAttribute('position', pa); geo.setAttribute('aV', va); geo.setAttribute('aI', ia); geo.setIndex(xa); geo.setDrawRange(0, 0);
  var mat = new T.ShaderMaterial({ vertexShader: STARFALL_RIB_VS, fragmentShader: STARFALL_RIB_FS, transparent: true, depthWrite: false, depthTest: false, side: T.DoubleSide, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    uniforms: { uTime: { value: 0 }, uCover: { value: cover }, uGlow: { value: glow }, uCore: { value: new T.Color('#f4f8ff') }, uIce: { value: new T.Color(STARFALL.ice) }, uGold: { value: new T.Color(STARFALL.gold) }, uDark: { value: new T.Color(STARFALL.dark) } } });
  var mesh = new T.Mesh(geo, mat); mesh.frustumCulled = false; mesh.renderOrder = order; mesh.visible = false;
  var MAXP = 40, P = new Float32Array(MAXP * 3), W = new Float32Array(MAXP), I = new Float32Array(MAXP), A = new Float32Array(MAXP), nv = 0, ni = 0;
  return { mesh: mesh, mat: mat, P: P, W: W, I: I, A: A,
    begin: function () { nv = 0; ni = 0; },
    push: function (n, cf, hue, Lw) {
      if (n < 2 || nv + n * 2 > NV) return;
      var base = nv, lx = 0, lz = 1, i;
      for (i = 0; i < n; i++) {
        var a = (i > 0 ? i - 1 : 0) * 3, b = (i < n - 1 ? i + 1 : n - 1) * 3, p = i * 3;
        var tx = P[b] - P[a], tz = P[b + 2] - P[a + 2], tl = Math.sqrt(tx * tx + tz * tz), nx, nz;
        if (tl > 1e-6) { nx = -tz / tl; nz = tx / tl; lx = nx; lz = nz; } else { nx = lx; nz = lz; }
        var w = W[i], q = nv * 3, r = nv * 2, c = nv * 4;
        pos[q] = P[p] - nx * w; pos[q + 1] = P[p + 1]; pos[q + 2] = P[p + 2] - nz * w;
        pos[q + 3] = P[p] + nx * w; pos[q + 4] = P[p + 1]; pos[q + 5] = P[p + 2] + nz * w;
        av[r] = A[i]; av[r + 1] = -1; av[r + 2] = A[i]; av[r + 3] = 1;
        ai[c] = I[i]; ai[c + 1] = cf; ai[c + 2] = hue; ai[c + 3] = Lw; ai[c + 4] = I[i]; ai[c + 5] = cf; ai[c + 6] = hue; ai[c + 7] = Lw;
        nv += 2;
      }
      for (i = 0; i < n - 1; i++) { var k = base + i * 2; ix[ni++] = k; ix[ni++] = k + 1; ix[ni++] = k + 2; ix[ni++] = k + 1; ix[ni++] = k + 3; ix[ni++] = k + 2; }
    },
    // A constellation line from (ax,ay,az) to (bx,by,bz): a capsule, half-width w.
    line: function (ax, ay, az, bx, by, bz, w, inten, hue) {
      var dx = bx - ax, dz = bz - az, l = Math.sqrt(dx * dx + dz * dz);
      if (l < 1e-5) return;
      var ux = dx / l, uz = dz / l;
      P[0] = ax - ux * w; P[1] = ay; P[2] = az - uz * w; P[3] = ax; P[4] = ay; P[5] = az; P[6] = bx; P[7] = by; P[8] = bz; P[9] = bx + ux * w; P[10] = by; P[11] = bz + uz * w;
      var Lw = l / w; W[0] = W[1] = W[2] = W[3] = w; I[0] = I[1] = I[2] = I[3] = inten; A[0] = -1; A[1] = 0; A[2] = Lw; A[3] = Lw + 1;
      this.push(4, 0.3, hue, Lw);
    },
    end: function () { geo.setDrawRange(0, ni); pa.needsUpdate = va.needsUpdate = ia.needsUpdate = xa.needsUpdate = true; mesh.visible = ni > 0; }
  };
}

// A comet's path: a quadratic curve from a to b with its control point c, as nine numbers.
function starfallPath(ax, ay, az, bx, by, bz, bend) {
  var mx = (ax + bx) / 2, mz = (az + bz) / 2, dx = bx - ax, dz = bz - az, l = Math.sqrt(dx * dx + dz * dz) || 1;
  return new Float32Array([ax, ay, az, mx - dz / l * l * bend, (ay + by) / 2 + l * 0.06, mz + dx / l * l * bend, bx, by, bz]);
}
function starfallAt(cm, u, out) {
  var a = 1 - u, w0 = a * a, w1 = 2 * a * u, w2 = u * u;
  out[0] = w0 * cm[0] + w1 * cm[3] + w2 * cm[6]; out[1] = w0 * cm[1] + w1 * cm[4] + w2 * cm[7]; out[2] = w0 * cm[2] + w1 * cm[5] + w2 * cm[8];
}
var STARFALL_TMP = [0, 0, 0];
// A comet's trail into the ribbon mesh: the head at path fraction uHead, the tail `tail` of the path
// behind it, tapering to nothing. K+1 points.
function starfallTrail(rb, cm, uHead, tail, w0, I0, hue, K, cf) {
  if (uHead <= 0.002 || tail <= 0.001) return;
  var P = rb.P, W = rb.W, I = rb.I, A = rb.A, tmp = STARFALL_TMP;
  for (var i = 0; i <= K; i++) {
    var f = i / K, u = uHead - tail * f; if (u < 0) u = 0;
    starfallAt(cm, u, tmp); P[i * 3] = tmp[0]; P[i * 3 + 1] = tmp[1]; P[i * 3 + 2] = tmp[2];
    W[i] = w0 * (Math.pow(1 - f, 0.7) + 0.03); I[i] = I0 * Math.pow(1 - f, 1.25); A[i] = 0;
  }
  rb.push(K + 1, cf, hue, -1);
}
// The comet's head: a short fat capsule at the front of the trail, so the head still shows as a bright
// blob with a dark rim where there is no night sky behind it.
var STARFALL_TMP2 = [0, 0, 0];
function starfallHead(rb, cm, u, w, inten, hue) {
  var a = STARFALL_TMP, b = STARFALL_TMP2;
  starfallAt(cm, u, a); starfallAt(cm, Math.max(0, u - 0.025), b);
  rb.line(b[0], b[1], b[2], a[0], a[1], a[2], w, inten, hue);
}

// ---------- Stars, landing rings and flashes: one instanced mesh of flat quads ----------
// Kind 0 is a lens star: a hot point with a halo and four long thin rays and four short diagonal ones,
// `au` the rays' length. Kind 1 is a landing: a thin ring at 0.78 of the quad that widens as it goes,
// `au` its softness and `ab` the strength of a soft flash disc under it. Both add light.
var STARFALL_STAR_VS = [
  'attribute vec4 aA; attribute vec4 aB; varying vec2 vUv; varying vec4 vA; varying vec4 vB;',
  'void main(){ vUv = position.xz; vA = aA; vB = aB; gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0); }'].join('\n');
var STARFALL_STAR_FS = [
  'uniform float uTime; varying vec2 vUv; varying vec4 vA; varying vec4 vB;',
  'void main(){',
  ' float kind = vA.x, al = vA.y, ph = vA.z, au = vA.w; vec3 tint = vB.xyz;',
  ' if (al <= 0.003) discard;',
  ' vec2 q = vUv; float r = length(q); vec3 col;',
  ' if (kind < 0.5) {',
  '  float tw = 0.88 + 0.12 * sin(uTime * 8.0 + ph * 6.2832);',
  '  float m = 1.0 - smoothstep(0.55, 1.0, r);',
  '  float core = exp(-r * r * 85.0), halo = exp(-r * r * 10.0) * 0.5 * m, soft = exp(-r * r * 3.0) * 0.16 * m;',
  '  vec2 a = abs(q), d = vec2(q.x + q.y, q.x - q.y) * 0.7071, b = abs(d);',
  '  float rx = exp(-a.y * 34.0) * (1.0 - smoothstep(0.0, au, a.x)), ry = exp(-a.x * 34.0) * (1.0 - smoothstep(0.0, au, a.y));',
  '  float rd = (exp(-b.y * 42.0) * (1.0 - smoothstep(0.0, au * 0.55, b.x)) + exp(-b.x * 42.0) * (1.0 - smoothstep(0.0, au * 0.55, b.y))) * 0.6;',
  '  col = tint * (core * 2.4 + halo + soft + (rx + ry + rd) * 0.95 * tw) * al + vec3(1.0) * core * 1.3 * al;',
  ' } else {',
  '  float w = 0.026 + 0.016 * (1.0 - au), ring = exp(-pow((r - 0.78) / w, 2.0)), edge = 1.0 - smoothstep(0.86, 1.0, max(abs(q.x), abs(q.y)));',
  '  col = tint * (ring * al * 1.2 + vB.w * exp(-r * r * 8.0)) * edge;',
  ' }',
  ' gl_FragColor = vec4(col, 1.0);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function starfallStars(st, n, order) {
  var T = st.T, geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute([-1, 0, -1, 1, 0, -1, -1, 0, 1, 1, 0, 1], 3)); geo.setIndex([0, 2, 1, 1, 2, 3]);
  var aA = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4), aB = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4);
  aA.setUsage(T.DynamicDrawUsage); aB.setUsage(T.DynamicDrawUsage); geo.setAttribute('aA', aA); geo.setAttribute('aB', aB);
  // Light only: it adds, and leaves the canvas's own alpha alone, like the glow layers on the dice.
  var mat = new T.ShaderMaterial({ vertexShader: STARFALL_STAR_VS, fragmentShader: STARFALL_STAR_FS, uniforms: { uTime: { value: 0 } }, transparent: true, depthWrite: false, depthTest: false, side: T.DoubleSide, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor });
  var mesh = new T.InstancedMesh(geo, mat, n); mesh.instanceMatrix.setUsage(T.DynamicDrawUsage); mesh.frustumCulled = false; mesh.renderOrder = order;
  var M = mesh.instanceMatrix.array, A = aA.array, B = aB.array, i;
  for (i = 0; i < M.length; i++) M[i] = 0;
  return { mesh: mesh, mat: mat,
    // size: the quad's half-width in world units.
    set: function (i, x, y, z, size, kind, c, alpha, ph, au, ab) {
      var m = i * 16, j = i * 4;
      M[m] = size; M[m + 5] = 1; M[m + 10] = size; M[m + 15] = 1; M[m + 12] = x; M[m + 13] = y; M[m + 14] = z;
      A[j] = kind; A[j + 1] = alpha; A[j + 2] = ph; A[j + 3] = au; B[j] = c[0]; B[j + 1] = c[1]; B[j + 2] = c[2]; B[j + 3] = ab;
    },
    hide: function (i) { var m = i * 16; M[m] = M[m + 5] = M[m + 10] = M[m + 15] = 0; A[i * 4 + 1] = 0; },
    commit: function (time) { mesh.instanceMatrix.needsUpdate = true; aA.needsUpdate = true; aB.needsUpdate = true; mat.uniforms.uTime.value = time; }
  };
}

// ---------- Dark patches under the echoes ----------
// A soft dark disc on the page under a die, so that light read on a white page; drawn under the dice
// (they are opaque), so it never covers the die itself.
var STARFALL_DIM_VS = 'attribute float aA; varying vec2 vUv; varying float vA; void main(){ vUv = position.xz; vA = aA; gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(position, 1.0); }';
var STARFALL_DIM_FS = [
  'varying vec2 vUv; varying float vA;',
  'void main(){ float f = 1.0 - smoothstep(0.1, 1.0, length(vUv)); float a = vA * f * f; if (a <= 0.003) discard;',
  ' gl_FragColor = vec4(vec3(0.004, 0.008, 0.032) * a, a);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function starfallDims(st, n) {
  var T = st.T, geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute([-1, 0, -1, 1, 0, -1, -1, 0, 1, 1, 0, 1], 3)); geo.setIndex([0, 2, 1, 1, 2, 3]);
  var aA = new T.InstancedBufferAttribute(new Float32Array(n), 1); aA.setUsage(T.DynamicDrawUsage); geo.setAttribute('aA', aA);
  var mat = new T.ShaderMaterial({ vertexShader: STARFALL_DIM_VS, fragmentShader: STARFALL_DIM_FS, transparent: true, depthWrite: false, side: T.DoubleSide, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor });
  var mesh = new T.InstancedMesh(geo, mat, n); mesh.instanceMatrix.setUsage(T.DynamicDrawUsage); mesh.frustumCulled = false; mesh.renderOrder = -2;
  var M = mesh.instanceMatrix.array, A = aA.array;
  for (var i = 0; i < M.length; i++) M[i] = 0;
  return { mesh: mesh,
    set: function (i, x, z, size, alpha) { var m = i * 16; M[m] = size; M[m + 5] = 1; M[m + 10] = size; M[m + 15] = 1; M[m + 12] = x; M[m + 13] = 0.012; M[m + 14] = z; A[i] = alpha; },
    hide: function (i) { var m = i * 16; M[m] = M[m + 5] = M[m + 10] = M[m + 15] = 0; A[i] = 0; },
    commit: function () { mesh.instanceMatrix.needsUpdate = true; aA.needsUpdate = true; } };
}

// ---------- Sparks: one Points, each with its own size ----------
var STARFALL_SPARK_VS = [
  'uniform float uH; attribute vec4 aCol; attribute float aSz; varying vec4 vCol;',
  'void main(){ vec4 mv = modelViewMatrix * vec4(position, 1.0); gl_Position = projectionMatrix * mv; vCol = aCol; gl_PointSize = max(1.0, aSz * uH * 0.5 / (' + TAN.toFixed(5) + ' * -mv.z)); }'].join('\n');
var STARFALL_SPARK_FS = [
  'varying vec4 vCol;',
  'void main(){',
  ' vec2 c = gl_PointCoord * 2.0 - 1.0; float r = length(c);',
  ' float s = exp(-r * r * 7.0) + 0.55 * (exp(-abs(c.x) * 18.0) * exp(-abs(c.y) * 2.2) + exp(-abs(c.y) * 18.0) * exp(-abs(c.x) * 2.2));',
  ' if (vCol.a <= 0.003) discard;',
  ' gl_FragColor = vec4(vCol.rgb * s * vCol.a * 1.5, 1.0);',
  ' #include <colorspace_fragment>',
  '}'].join('\n');
function starfallSparks(st, n, order) {
  var T = st.T, pos = new Float32Array(n * 3), col = new Float32Array(n * 4), sz = new Float32Array(n);
  var vx = new Float32Array(n), vy = new Float32Array(n), vz = new Float32Array(n), age = new Float32Array(n), life = new Float32Array(n), s0 = new Float32Array(n), dg = new Float32Array(n);
  var cr = new Float32Array(n), cg = new Float32Array(n), cb = new Float32Array(n), next = 0, i;
  for (i = 0; i < n; i++) { age[i] = 1; life[i] = 1; }
  var geo = new T.BufferGeometry(), pa = new T.BufferAttribute(pos, 3), ca = new T.BufferAttribute(col, 4), sa = new T.BufferAttribute(sz, 1);
  [pa, ca, sa].forEach(function (b) { b.setUsage(T.DynamicDrawUsage); });
  geo.setAttribute('position', pa); geo.setAttribute('aCol', ca); geo.setAttribute('aSz', sa);
  var mat = new T.ShaderMaterial({ vertexShader: STARFALL_SPARK_VS, fragmentShader: STARFALL_SPARK_FS, uniforms: { uH: { value: 800 } }, transparent: true, depthWrite: false, depthTest: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneFactor, blendSrcAlpha: T.ZeroFactor, blendDstAlpha: T.OneFactor });
  var pts = new T.Points(geo, mat); pts.frustumCulled = false; pts.renderOrder = order;
  return { mesh: pts,
    emit: function (x, y, z, ux, uy, uz, lf, size, c, drag) {
      var k = next; next = (next + 1) % n;
      pos[k * 3] = x; pos[k * 3 + 1] = y; pos[k * 3 + 2] = z; vx[k] = ux; vy[k] = uy; vz[k] = uz; age[k] = 0; life[k] = lf; s0[k] = size; dg[k] = drag; cr[k] = c[0]; cg[k] = c[1]; cb[k] = c[2];
    },
    step: function (dt) {
      mat.uniforms.uH.value = st.H * st.renderer.getPixelRatio();
      for (var k = 0; k < n; k++) {
        if (age[k] >= life[k]) { col[k * 4 + 3] = 0; continue; }
        age[k] += dt;
        var f = age[k] / life[k];
        if (f >= 1) { col[k * 4 + 3] = 0; continue; }
        var j = k * 3, d = Math.max(0, 1 - dg[k] * dt);
        vx[k] *= d; vy[k] *= d; vz[k] *= d; pos[j] += vx[k] * dt; pos[j + 1] += vy[k] * dt; pos[j + 2] += vz[k] * dt;
        // Hot white first, then its own colour as it cools.
        var hot = Math.max(0, 1 - f * 2.2), q = k * 4;
        col[q] = cr[k] + (1 - cr[k]) * hot; col[q + 1] = cg[k] + (1 - cg[k]) * hot; col[q + 2] = cb[k] + (1 - cb[k]) * hot;
        col[q + 3] = Math.min(1, f * 12) * Math.pow(1 - f, 1.3) * (0.75 + 0.25 * Math.sin(age[k] * 40 + k));
        sz[k] = s0[k] * (1 - 0.5 * f);
      }
      pa.needsUpdate = ca.needsUpdate = sa.needsUpdate = true;
    }
  };
}

function starfallRGB(T, hex) { var c = new T.Color(hex); return [c.r, c.g, c.b]; }
function starfallLerp(a, b, k) { return [a[0] + (b[0] - a[0]) * k, a[1] + (b[1] - a[1]) * k, a[2] + (b[2] - a[2]) * k]; }

// ---------- The grand ----------
function fxStarfall(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, frame = 0, i, j;
  // A new shower every time: where the comets come from and how they bend.
  var rnd = mulberry(5309 + Math.floor(Math.random() * 1e6));
  g.updateMatrixWorld(true);
  var p0 = g.position.clone(), top = worldOf(die, die.faces[fi].c), clear = (o && o.clear) || [];
  var END = 4.7, TF = 3.0;
  // Light only adds, so on a pale stone a silver number washes out: there the number keeps the set's own
  // lit colour, and the flare is held lower (PK) so it does not bleach the face.
  var pale = th && th.base && th.base.grad ? isLight(th) : false, NUM = pale && th.burn ? th.burn : STARFALL.num, PK = pale ? 0.62 : 1;

  // ----- The crown, placed and kept off the other dice -----
  var fg = STARFALL_CROWN, R = starfallFit(st, p0.x, p0.z, s), S = starfallCrownAt(p0.x, p0.z, R, []), N = S.length;
  for (var pass = 0; pass < 2; pass++) S.forEach(function (q) {
    clear.forEach(function (c) {
      var dx = q.x - c.x, dz = q.z - c.z, d = Math.sqrt(dx * dx + dz * dz) || 1e-3, lim = c.r * 0.85;
      if (d < lim) { q.x = c.x + dx / d * lim; q.z = c.z + dz / d * lim; }
    });
  });
  die.starfallFig = S;
  // The sky is centred on the crown, which rides above the die, and is wide enough to hold all of it.
  var C = { x: p0.x, z: p0.z - 0.24 * R }, R0 = R * 2.45;
  var iLast = fg.order[N - 1], last = S[iLast];

  // ----- When each star lands: a shower that quickens, then a pause for the highest jewel -----
  var land = [], dur = [], cms = [], Lp = [], tint = [], seeds = [], shed = [], landed = [], gone = [];
  var wsum = [0], acc = 0;
  for (j = 0; j < N - 2; j++) { acc += 1 - 0.45 * j / (N - 3); wsum.push(acc); }
  for (j = 0; j < N - 1; j++) { land[fg.order[j]] = 0.86 + 1.76 * wsum[j] / wsum[N - 2]; dur[fg.order[j]] = 0.5 + 0.12 * rnd() + (j < 3 ? 0.1 : 0); }
  land[iLast] = TF; dur[iLast] = 0.8;
  var white = starfallRGB(T, STARFALL.white), ice = starfallLerp(white, starfallRGB(T, STARFALL.ice), 0.6), gold = starfallLerp(white, starfallRGB(T, STARFALL.gold), 0.75);
  function planComet(i) {
    var q = S[i], th0 = Math.atan2(q.z - C.z, q.x - C.x), cand = i === iLast ? [0] : [0.6, 0.3, 0.9, -0.3, -0.6, 0, 1.2], bs = -1e9, ax = 0, az = 0;
    for (var k = 0; k < cand.length; k++) {
      var ang = i === iLast ? -Math.PI / 2 + 0.38 : th0 + cand[k] + (rnd() - 0.5) * 0.16, Rs = R0 * (0.74 + 0.2 * rnd());
      var px = C.x + Math.cos(ang) * Rs, pz = C.z + Math.sin(ang) * Rs, sc = 1e9;
      for (var m = 0; m < clear.length; m++) sc = Math.min(sc, starfallSegDist(px, pz, q.x, q.z, clear[m].x, clear[m].z) - clear[m].r * 0.9);
      if (sc > bs) { bs = sc; ax = px; az = pz; }
      if (sc > 0) break;
    }
    // Falling: it starts high and comes down onto the page, so the streak is a slant, not a slide.
    var ay = s * (1.0 + 1.3 * rnd());
    cms[i] = starfallPath(ax, ay, az, q.x, 0.05, q.z, (rnd() < 0.5 ? -1 : 1) * (0.05 + 0.07 * rnd()));
    Lp[i] = Math.sqrt((ax - q.x) * (ax - q.x) + (az - q.z) * (az - q.z)) * 1.03;
  }
  for (i = 0; i < N; i++) {
    planComet(i); landed.push(false); shed.push(0); seeds.push(rnd());
    tint.push(starfallLerp(ice, gold, S[i].h)); gone.push(rnd());
  }

  // ----- What goes in the scene -----
  var sky = starfallSky(st, hd, C, R0, clear);
  var rb = starfallRibbons(st, hd ? 1400 : 900, 8, 1.2, 1), stars = starfallStars(st, 2 * N + 1, 10), sp = starfallSparks(st, hd ? 420 : 200, 11);
  st.scene.add(sky.mesh, rb.mesh, stars.mesh, sp.mesh);
  var light = hd ? new T.PointLight(0xdbe8ff, 0, s * 15, 1.3) : null; if (light) st.scene.add(light);
  var gf = glowFace(die, fi); gf.clear(); numberOnto(gf.g, gf.lay, NUM); gf.commit(0);
  var fcl = glowLayer(die, fi), SILVER = starfallRGB(T, STARFALL.silver), POS = [0, 0, 0];
  var K = hd ? 14 : 9, lk = 0, lastP = { x: p0.x, z: p0.z }, flared = false, flareT = 0;

  function burst(x, y, z, n, speed, size, c, life) {
    n = hd ? n : Math.ceil(n / 2);
    for (var k = 0; k < n; k++) {
      var a = rnd() * 6.2832, v = speed * (0.35 + rnd() * 0.85), cc = rnd() < 0.3 ? gold : (rnd() < 0.5 ? c : white);
      sp.emit(x, y, z, Math.cos(a) * v, speed * 0.25 * rnd(), Math.sin(a) * v, life * (0.55 + rnd() * 0.5), size * (0.6 + rnd() * 0.7), cc, 2.2);
    }
  }
  function onLand(i) {
    var q = S[i], fin = i === iLast;
    lk = 1; lastP.x = q.x; lastP.z = q.z;
    burst(q.x, 0.05, q.z, fin ? 46 : 18, s * (fin ? 5.5 : 3.4), s * 0.2, tint[i], 0.85);
  }

  return { update: function (dt) {
    t += dt; frame++;
    if (t >= END) {
      drop(st, [sky.mesh, rb.mesh, stars.mesh, sp.mesh]); stars.mesh.dispose();
      if (light) { st.scene.remove(light); light.dispose(); }
      die.starfallFig = null;
      glowNumeral(die, fi, NUM); shadeDie(die, 1); tintDie(die, th.burn, 0);
      return false;
    }
    lk *= Math.exp(-dt * 7);
    var night = smooth(0.05, 0.8, t) * (1 - smooth(3.5, 4.4, t));
    var fl = t > TF + 0.08 ? env4(t, TF + 0.08, TF + 0.2, TF + 0.42, TF + 1.4) : 0;
    var lineFade = 1 - smooth(3.6, 4.3, t);

    // The sky opens like an iris, darkens and fills with stars; it lightens a little in the flare.
    var U = sky.uni; U.uTime.value = t; U.uOp.value = smooth(0, 0.55, t) * (1 - smooth(3.5, 4.5, t)); U.uR.value = R0 * (0.8 + 0.2 * out3(t / 0.8));
    U.uFlash.value = 0.45 * fl;

    rb.begin();
    for (i = 0; i < N; i++) {
      var q = S[i], ls = land[i], D = dur[i], tau = (t - (ls - D)) / D, dl = t - ls, fin = i === iLast;
      var h0 = 2 * i, h1 = 2 * i + 1;
      if (tau <= 0) { stars.hide(h0); stars.hide(h1); continue; }
      if (!landed[i] && dl >= 0) { landed[i] = true; onLand(i); }
      var dd = Math.sqrt((q.x - last.x) * (q.x - last.x) + (q.z - last.z) * (q.z - last.z)), tf = TF + 0.1 + dd / (R * 5.5);
      var pulse = Math.exp(-Math.pow((t - tf) / 0.13, 2)) * (t > TF ? 1 : 0);
      var fade = 1 - smooth(3.55 + 0.35 * gone[i], 4.05 + 0.4 * gone[i], t), tt = tint[i], w = q.w;
      if (dl < 0) {
        // In flight: the head and its trail.
        var u = 0.22 * tau + 0.78 * tau * tau, cm = cms[i];
        starfallAt(cm, u, POS);
        var tail = Math.min(u, s * (fin ? 4.4 : 2.7) / Lp[i]);
        var cI = smooth(0.04, 0.36, tau) * (fin ? 1.15 : 1);
        starfallTrail(rb, cm, u, tail, s * (fin ? 0.24 : 0.17), cI, S[i].h, K, 0.24);
        starfallHead(rb, cm, u, s * (fin ? 0.16 : 0.1), cI, S[i].h);
        stars.set(h0, POS[0], POS[1] + 0.02, POS[2], s * (fin ? 0.8 : 0.5) * (0.6 + 0.4 * tau), 0, tt, smooth(0, 0.12, tau), seeds[i], 0.5, 0);
        stars.hide(h1);
        shed[i] += dt * (hd ? 46 : 22);
        while (shed[i] >= 1) { shed[i] -= 1; sp.emit(POS[0], POS[1], POS[2], (rnd() - 0.5) * s * 1.4, rnd() * s * 0.5, (rnd() - 0.5) * s * 1.4, 0.35 + rnd() * 0.3, s * 0.06, tt, 2.5); }
      } else {
        // Landed: the trail runs into the point, the star pops out and stays, a ring and a flash go out.
        if (dl < 0.26) {
          var shrink = 1 - smooth(0, 0.24, dl);
          starfallTrail(rb, cms[i], 1, Math.min(1, s * (fin ? 4.4 : 2.7) / Lp[i]) * shrink, s * (fin ? 0.24 : 0.17), shrink, S[i].h, K, 0.24);
        }
        var s0 = s * (0.5 + 0.3 * w), grow = starfallBack(dl / 0.28), size = s0 * grow * (1 + 0.45 * pulse + 0.7 * Math.exp(-dl / 0.12)) * (0.4 + 0.6 * fade);
        var au = clamp01(0.55 + 0.2 * w + 0.3 * pulse + 0.4 * Math.exp(-dl / 0.15));
        stars.set(h0, q.x, 0.05, q.z, size, 0, tt, fade, seeds[i], au, 0);
        if (dl < 0.6) {
          var p = dl / 0.55, rr = s * (0.5 + (fin ? 2.0 : 0.6) * out3(p)) / 0.78, fa = Math.pow(1 - clamp01(p), 1.5) * (fin ? 0.8 : 0.5), fd = dl < 0.3 ? Math.pow(1 - dl / 0.3, 2) * (fin ? 1.4 : 0.9) : 0;
          stars.set(h1, q.x, 0.05, q.z, rr, 1, tt, fa, 0, clamp01(p), fd);
        } else stars.hide(h1);
      }
    }
    // The constellation's lines draw themselves from the star that landed first to the one that landed after.
    var Ls = fg.l;
    for (i = 0; i < Ls.length; i++) {
      var ln = Ls[i], a = ln[0], b = ln[1], f0 = land[a] <= land[b] ? a : b, f1 = f0 === a ? b : a, pr = clamp01((t - land[f1] - 0.03) / 0.2);
      if (pr <= 0) continue;
      pr = out3(pr);
      var A0 = S[f0], A1 = S[f1], ex = A0.x + (A1.x - A0.x) * pr, ez = A0.z + (A1.z - A0.z) * pr;
      var md = Math.sqrt(Math.pow((A0.x + A1.x) / 2 - last.x, 2) + Math.pow((A0.z + A1.z) / 2 - last.z, 2)), lp = Math.exp(-Math.pow((t - TF - 0.1 - md / (R * 5.5)) / 0.14, 2)) * (t > TF ? 1 : 0);
      var inten = ((ln[2] ? 0.55 : 0.85) * (1 + 0.15 * smooth(TF, TF + 0.4, t)) + 0.9 * lp) * lineFade;
      rb.line(A0.x, 0.04, A0.z, ex, 0.04, ez, s * 0.06 * (1 + 0.3 * lp), inten, 0.1);
      if (pr < 1 && hd) sp.emit(ex, 0.05, ez, (rnd() - 0.5) * s * 0.8, rnd() * s * 0.3, (rnd() - 0.5) * s * 0.8, 0.3, s * 0.055, ice, 2.5);
    }
    rb.end(); rb.mat.uniforms.uTime.value = t;

    // The flare: a ring of silver runs out through the crown from the die, and the die flares white.
    if (!flared && t >= TF + 0.08) {
      flared = true; flareT = t;
      burst(p0.x, top.y, p0.z, 60, s * 6, s * 0.2, ice, 1.0);
    }
    var ix = 2 * N;
    if (flared) {
      var fk = (t - flareT) / 1.0;
      if (fk < 1) stars.set(ix, p0.x, 0.06, p0.z, s * (0.8 + 4.4 * out3(fk)) / 0.78, 1, SILVER, Math.pow(1 - fk, 2.2) * 0.5, 0, clamp01(fk), Math.pow(1 - clamp01(fk * 3), 2) * 0.5);
      else stars.hide(ix);
    } else stars.hide(ix);
    sp.step(dt); stars.commit(t);

    // The die: moonlit under the sky, a little lit by each landing, silver-white in the flare.
    shadeDie(die, 1 - (pale ? 0.2 : 0.1) * night - 0.18 * fl);
    tintDie(die, STARFALL.tint, (0.1 * night + 0.5 * lk + 0.85 * fl) * (pale ? 0.3 : 1));
    fcl.mat.emissiveIntensity = 0.85 * smooth(0.1, 0.7, t) + 0.75 * smooth(TF + 0.7, 4.3, t) + (0.7 * lk + 1.9 * fl) * PK;
    if (light) {
      light.position.set(p0.x + (lastP.x - p0.x) * 0.5, top.y + s * 1.2, p0.z + (lastP.z - p0.z) * 0.5);
      light.intensity = (4 * night + 36 * lk + 100 * fl) * (pale ? 0.5 : 1);
    }
    return true;
  } };
}

// ---------- The echo ----------
// One shooting star from the grand's crown (or from the page edge when there is no grand) to the top
// of this die: a trail, a small ring and flash, a few sparks, the number lit. Every echo of a throw
// shares one ribbon mesh, one star mesh and one Points (the hub), so a dozen of them still cost three
// draws; each echo is a small job the hub steps. Its pieces draw above anything the grand put down.
function starfallHub(st) {
  var hd = !(st.q && st.q.detail === 'basic'), NJ = 14;
  var rb = starfallRibbons(st, hd ? 700 : 450, 68, 1.9, 0.5), stars = starfallStars(st, NJ * 2, 70), sp = starfallSparks(st, hd ? 360 : 180, 71), dims = starfallDims(st, NJ);
  st.scene.add(rb.mesh, stars.mesh, sp.mesh, dims.mesh);
  var hub = { jobs: [], rb: rb, stars: stars, sp: sp, dims: dims, alive: true, t: 0, hd: hd, NJ: NJ };
  st.fx.push({ update: function (dt) {
    hub.t += dt; rb.begin();
    var live = false;
    for (var k = 0; k < hub.jobs.length; k++) { var jb = hub.jobs[k]; if (jb.alive) { jb.step(dt); if (jb.alive) live = true; } }
    rb.end(); rb.mat.uniforms.uTime.value = hub.t; sp.step(dt); stars.commit(hub.t); dims.commit();
    if (live) return true;
    hub.alive = false; if (st.starfallHub === hub) st.starfallHub = null;
    drop(st, [rb.mesh, stars.mesh, sp.mesh, dims.mesh]); stars.mesh.dispose(); dims.mesh.dispose();
    return false;
  } });
  return hub;
}

function fxStarfallEcho(st, die, fi, th, e) {
  var T = st.T, s = die.size, hub = st.starfallHub && st.starfallHub.alive ? st.starfallHub : (st.starfallHub = starfallHub(st)), hd = hub.hd;
  die.group.updateMatrixWorld(true);
  var p0 = die.group.position, top = worldOf(die, die.faces[fi].c), rnd = mulberry(7717 + Math.floor(Math.random() * 1e6));
  var pale = th && th.base && th.base.grad ? isLight(th) : false, NUM = pale && th.burn ? th.burn : STARFALL.num, PK = pale ? 0.62 : 1;
  // Where the star starts: the crown star nearest this die, or the page edge when no crown is up.
  var ox, oy, oz, from = e && e.from, fig = from && from.starfallFig;
  if (from && fig && fig.length) {
    var bd = 1e9, bi = 0;
    fig.forEach(function (q, k) { var d = (q.x - p0.x) * (q.x - p0.x) + (q.z - p0.z) * (q.z - p0.z); if (d < bd) { bd = d; bi = k; } });
    ox = fig[bi].x; oz = fig[bi].z; oy = 0.08;
  } else if (from) {
    var gp = from.group.position, dx = p0.x - gp.x, dz = p0.z - gp.z, dl = Math.sqrt(dx * dx + dz * dz) || 1;
    ox = gp.x + dx / dl * from.size * 2.4; oz = gp.z + dz / dl * from.size * 2.4; oy = 0.08;
  } else {
    var ang = -Math.PI / 2 + (rnd() - 0.5) * 1.3, ux = Math.cos(ang), uz = Math.sin(ang);
    var tx = ux > 1e-3 ? (st.hw - p0.x) / ux : ux < -1e-3 ? (-st.hw - p0.x) / ux : 1e9, tz = uz > 1e-3 ? (st.hh - p0.z) / uz : uz < -1e-3 ? (-st.hh - p0.z) / uz : 1e9;
    var L = Math.min(tx, tz) + s * 2.2; ox = p0.x + ux * L; oz = p0.z + uz * L; oy = s * 3.4;
  }
  var tgx = top.x, tgy = top.y + s * 0.03, tgz = top.z, len = Math.sqrt((ox - tgx) * (ox - tgx) + (oz - tgz) * (oz - tgz));
  var cm = starfallPath(ox, oy, oz, tgx, tgy, tgz, (rnd() < 0.5 ? -1 : 1) * (0.06 + 0.06 * rnd()));
  // A star leaving the crown rises a little before it comes down on the die.
  if (from) cm[4] += s * 0.9;
  var D = 0.5 + Math.min(0.25, len * 0.016), Lp = len * 1.03 + 0.001, END = D + 1.0;
  var slot = hub.jobs.length % hub.NJ, h0 = slot * 2, h1 = h0 + 1;
  var white = starfallRGB(T, STARFALL.white), tint = starfallLerp(white, starfallRGB(T, STARFALL.ice), 0.55), POS = [0, 0, 0], SILVER = starfallRGB(T, STARFALL.silver);
  var gf = glowFace(die, fi); gf.clear(); numberOnto(gf.g, gf.lay, NUM); gf.commit(0);
  var fcl = glowLayer(die, fi), shed = 0, landed = false, seed = rnd();
  var job = { alive: true, t: 0, step: function (dt) {
    var t = job.t += dt, rb = hub.rb, stars = hub.stars, sp = hub.sp;
    if (t >= END) {
      fcl.mat.emissiveIntensity = 1.5; tintDie(die, th.burn, 0); stars.hide(h0); stars.hide(h1); hub.dims.hide(slot); job.alive = false; return;
    }
    hub.dims.set(slot, p0.x, p0.z, s * 2.4, 0.32 * smooth(0, 0.35, t) * (1 - smooth(D + 0.5, END, t)));
    var tau = clamp01(t / D), dl = t - D, u = 0.2 * tau + 0.8 * tau * tau;
    if (dl < 0) {
      starfallAt(cm, u, POS);
      var cI = smooth(0, 0.15, tau);
      starfallTrail(rb, cm, u, Math.min(u, s * 2.6 / Lp), s * 0.17, cI, 0.1, hd ? 10 : 7, 0.2);
      starfallHead(rb, cm, u, s * 0.1, cI, 0.1);
      stars.set(h0, POS[0], POS[1] + 0.02, POS[2], s * 0.42 * (0.7 + 0.3 * tau), 0, tint, smooth(0, 0.1, tau), seed, 0.5, 0);
      stars.hide(h1);
      shed += dt * (hd ? 36 : 18);
      while (shed >= 1) { shed -= 1; sp.emit(POS[0], POS[1], POS[2], (rnd() - 0.5) * s * 1.2, rnd() * s * 0.4, (rnd() - 0.5) * s * 1.2, 0.3 + rnd() * 0.25, s * 0.055, tint, 2.5); }
    } else {
      if (!landed) {
        landed = true;
        for (var k = hd ? 14 : 8; k > 0; k--) { var a = rnd() * 6.2832, v = s * (1 + rnd() * 2.6); sp.emit(tgx, tgy, tgz, Math.cos(a) * v, rnd() * s, Math.sin(a) * v, 0.35 + rnd() * 0.4, s * (0.05 + rnd() * 0.05), rnd() < 0.3 ? SILVER : tint, 2.4); }
      }
      if (dl < 0.26) starfallTrail(rb, cm, 1, Math.min(1, s * 2.6 / Lp) * (1 - smooth(0, 0.24, dl)), s * 0.17, 1 - smooth(0, 0.24, dl), 0.1, hd ? 10 : 7, 0.2);
      stars.set(h0, tgx, tgy, tgz, s * (0.6 + 0.4 * Math.exp(-dl / 0.1)), 0, SILVER, (1 - smooth(0, 0.6, dl)) * (0.6 + 0.4 * PK), seed, 0.8, 0);
      if (dl < 0.55) { var p = dl / 0.5; stars.set(h1, tgx, tgy, tgz, s * (0.4 + 0.75 * out3(p)) / 0.78, 1, tint, Math.pow(1 - clamp01(p), 1.6) * 0.5, 0, clamp01(p), dl < 0.2 ? Math.pow(1 - dl / 0.2, 2) * 0.45 * PK : 0); }
      else stars.hide(h1);
    }
    fcl.mat.emissiveIntensity = dl < 0 ? 0.5 * smooth(D - 0.3, D, t) : 1.5 + (0.6 * Math.exp(-dl / 0.45) + 0.7 * Math.exp(-dl / 0.08)) * PK;
    tintDie(die, STARFALL.tint, dl < 0 ? 0 : 0.35 * (pale ? 0.5 : 1) * Math.exp(-dl / 0.25));
  } };
  hub.jobs.push(job);
  return { update: function () { return job.alive; } };
}
registerCrit('starfall', { name: 'Starfall', kind: 'succ',
  what: 'Night falls over the page round the die and shooting stars land round it, each staying as a bright point, until fine lines of light join them into a crown of stars that flares, lights the die silver-white and fades, leaving the 20 shining.',
  fx: fxStarfall, beat: 3.0, peak: 3.4, echo: fxStarfallEcho,
  echoWhat: 'One shooting star streaks down from the crown of stars, or from the page edge, and lands on the die, lighting its number.' });
