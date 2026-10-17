// ---------- Crit fail: Murder of crows ----------
// A flock of crows sweeps in across the page from one edge in a tightening spiral, circles the die, and
// a few of them dive and peck at it; the die jolts, feathers fall, and the 1 bruises to violet-red. Then
// the flock bursts off the page and the last feathers settle and fade.
//
// Seen from straight above, a crow is its wings: every bird is one instance of a single mesh (body,
// head and beak, tail fan, two wings of layered feathers with the primaries splayed at the tips, legs)
// bent by a small skeleton in the vertex shader. The wing is a spine that is integrated outward from the
// shoulder, so a wingbeat is three numbers per bird (flap, tip lag, sweep) and the tips trail the stroke
// the way real feathers do. The same deformation runs in the shadow pass, so the flock's shadows sweep
// over the page with the birds. Plumage is lit like glossy black feathers with a blue-violet sheen that
// grows at grazing angles, which is what keeps a black bird readable on a dark page.
import {
  canvas, clamp01, drop, glowFace, glowLayer, hexA, mix, mulberry, numberOnto, out3, registerCrit,
  smooth, spray, tintDie, worldOf, share
} from '../engine.js';

var CROWS = { ink: '#07060c', sheenA: '#3a54e6', sheenB: '#a24ae0', beak: '#17161c', bruise: '#cf3b80', deep: '#6b1747', shade: '#06040c' };

// ---------- Geometry ----------
// A crow in its own frame: x forward, y up, z to the right wing, wingspan 2 and body about 1 long.
// Wing feathers are laid flat in the x/z plane (the "bind pose") and the shader bends them from there.
function crowsAcc() { return { pos: [], fu: [], bind: [], idx: [], n: 0 }; }
// aBind: part (1 body and head, 2 wing, 3 tail, 4 leg, 5 beak), side, weight, tone.
function crowsVert(A, x, y, z, u, v, part, w, tone) { A.pos.push(x, y, z); A.fu.push(u, v); A.bind.push(part, 1, w, tone); return A.n++; }

// Half-width along a feather (u: root 0, tip 1): narrow at the quill, widest a third along, tip rounded.
function crowsWidth(u, shape) {
  var grow = 0.55 + 0.45 * Math.min(1, u / 0.3);
  if (shape === 1) return grow * Math.sqrt(Math.max(0, 1 - Math.pow(u, 3)));
  if (shape === 2) return (0.8 + 0.2 * Math.min(1, u / 0.3)) * Math.pow(Math.max(0, 1 - Math.pow(u, 4)), 0.5);
  if (shape === 3) return grow * Math.pow(Math.max(0, 1 - Math.pow(u, 5)), 0.5);
  return grow * Math.pow(Math.max(0, 1 - Math.pow(u, 2.6)), 0.55);
}
// One feather: a strip of three vertices across (the quill raised, the vane edges dropped) lying in the
// x/z plane from root (x, z) along angle `ang`, curling by `curl` over its length.
function crowsFeather(A, o) {
  var seg = o.seg || 8, base = A.n, x = o.x, z = o.z, ds = o.len / seg, k, j;
  for (k = 0; k <= seg; k++) {
    var u = k / seg;
    if (k) { var am = o.ang + o.curl * (u - 0.5 / seg); x += Math.cos(am) * ds; z += Math.sin(am) * ds; }
    var a = o.ang + o.curl * u, hw = o.w * crowsWidth(u, o.shape || 0), nx = -Math.sin(a), nz = Math.cos(a), y = o.y + o.lift * u * u;
    for (j = -1; j <= 1; j++) crowsVert(A, x + nx * hw * j, y - 0.28 * hw * Math.abs(j), z + nz * hw * j, u, (j + 1) / 2, o.part, 0, o.tone);
  }
  for (k = 0; k < seg; k++) for (j = 0; j < 2; j++) {
    var a0 = base + k * 3 + j, b0 = a0 + 1, c0 = a0 + 3, d0 = a0 + 4;
    A.idx.push(a0, b0, c0, b0, d0, c0);
  }
}
// A loft along x through stations [x, y, half-width, half-height, weight], closed at both ends. The
// winding is checked against the outward direction and flipped if needed.
function crowsLoft(A, st, ns, part, tone) {
  var base = A.n, i, k, rings = [];
  for (i = 0; i < st.length; i++) {
    var S = st[i];
    for (k = 0; k < ns; k++) { var an = k / ns * Math.PI * 2; crowsVert(A, S[0], S[1] + S[3] * Math.sin(an), S[2] * Math.cos(an), i / (st.length - 1), k / ns, part, S[4], tone); }
  }
  var c0 = crowsVert(A, st[0][0] - 0.01, st[0][1], 0, 0, 0, part, st[0][4], tone), c1 = crowsVert(A, st[st.length - 1][0] + 0.012, st[st.length - 1][1], 0, 1, 0, part, st[st.length - 1][4], tone), tris = [];
  for (i = 0; i < st.length - 1; i++) for (k = 0; k < ns; k++) {
    var a = base + i * ns + k, b = base + i * ns + (k + 1) % ns, c = a + ns, d = b + ns;
    tris.push(a, c, b, b, c, d);
  }
  for (k = 0; k < ns; k++) { tris.push(c0, base + k, base + (k + 1) % ns); var e = base + (st.length - 1) * ns; tris.push(c1, e + (k + 1) % ns, e + k); }
  // Outward check on the first side triangle: its normal against the direction from the axis.
  var p = A.pos, t0 = tris[0], t1 = tris[1], t2 = tris[2];
  var ux = p[t1 * 3] - p[t0 * 3], uy = p[t1 * 3 + 1] - p[t0 * 3 + 1], uz = p[t1 * 3 + 2] - p[t0 * 3 + 2];
  var vx = p[t2 * 3] - p[t0 * 3], vy = p[t2 * 3 + 1] - p[t0 * 3 + 1], vz = p[t2 * 3 + 2] - p[t0 * 3 + 2];
  var nx = uy * vz - uz * vy, ny = uz * vx - ux * vz, nz = ux * vy - uy * vx;
  var ox = 0, oy = p[t0 * 3 + 1] - st[0][1], oz = p[t0 * 3 + 2];
  if (nx * ox + ny * oy + nz * oz < 0) for (i = 0; i < tris.length; i += 3) { var tmp = tris[i + 1]; tris[i + 1] = tris[i + 2]; tris[i + 2] = tmp; }
  for (i = 0; i < tris.length; i++) A.idx.push(tris[i]);
}
// A three-sided bar from p0 to p1 tapering from w0 to w1: legs and toes.
function crowsBar(A, p0, p1, w0, w1, side) {
  var base = A.n, dx = p1[0] - p0[0], dy = p1[1] - p0[1], dz = p1[2] - p0[2], L = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
  dx /= L; dy /= L; dz /= L;
  var ax = 0, ay = 0, az = 1; if (Math.abs(dz) > 0.9) { ax = 1; az = 0; }
  // Two directions across the bar.
  var sx = dy * az - dz * ay, sy = dz * ax - dx * az, sz = dx * ay - dy * ax, sl = Math.sqrt(sx * sx + sy * sy + sz * sz) || 1; sx /= sl; sy /= sl; sz /= sl;
  var tx = dy * sz - dz * sy, ty = dz * sx - dx * sz, tz = dx * sy - dy * sx;
  [[p0, w0], [p1, w1]].forEach(function (E) {
    for (var k = 0; k < 3; k++) {
      var an = k / 3 * Math.PI * 2 + 0.5, c = Math.cos(an) * E[1], s = Math.sin(an) * E[1];
      var v = crowsVert(A, E[0][0] + sx * c + tx * s, E[0][1] + sy * c + ty * s, E[0][2] + sz * c + tz * s, 0, 0.5, 4, side, 0.5);
    }
  });
  for (var k2 = 0; k2 < 3; k2++) { var a = base + k2, b = base + (k2 + 1) % 3, c2 = a + 3, d = b + 3; A.idx.push(a, c2, b, b, c2, d); }
}

// The right wing in the bind pose. Layers from the bottom: an underlay, the secondaries along the
// trailing edge, the seven primaries fanned out from the wrist, then three rows of coverts and a pad
// along the leading edge. Heights are a few thousandths apart so the layers never fight.
function crowsWing(A, rnd) {
  var i, PI = Math.PI;
  [0.16, 0.27, 0.38, 0.49].forEach(function (z) { crowsFeather(A, { x: 0.12, z: z, ang: PI - 0.05, len: 0.2, w: 0.075, y: 0, lift: 0, curl: 0, part: 2, tone: rnd(), seg: 4, shape: 2 }); });
  for (i = 0; i < 9; i++) {
    var ph = -0.12 + 0.37 * i / 8;
    crowsFeather(A, { x: 0.09 - 0.02 * i / 8, z: 0.14 + i * 0.0425, ang: PI - ph, len: 0.34 + 0.08 * Math.sin(PI * (0.15 + 0.85 * i / 8)), w: 0.047, y: 0.004 + 0.0007 * i, lift: 0.012, curl: -0.04, part: 2, tone: rnd(), seg: 8, shape: 2 });
  }
  var PSI = [0.98, 0.80, 0.63, 0.47, 0.32, 0.18, 0.05], PL = [0.40, 0.46, 0.52, 0.57, 0.55, 0.50, 0.45];
  for (i = 0; i < 7; i++) crowsFeather(A, { x: 0.10, z: 0.46 + 0.016 * i, ang: PI / 2 + PSI[i], len: PL[i], w: i > 4 ? 0.034 : 0.042, y: 0.009 + 0.001 * i, lift: 0.035, curl: 0.12, part: 2, tone: rnd(), seg: 10, shape: 3 });
  for (i = 0; i < 6; i++) crowsFeather(A, { x: 0.17, z: 0.5 + 0.016 * i, ang: PI / 2 + PSI[i] * 0.9, len: 0.24, w: 0.034, y: 0.018 + 0.0008 * i, lift: 0.02, curl: 0.08, part: 2, tone: rnd(), seg: 5, shape: 1 });
  for (i = 0; i < 10; i++) {
    var pg = -0.12 + 0.37 * i / 9;
    crowsFeather(A, { x: 0.15, z: 0.15 + 0.037 * i, ang: PI - pg, len: 0.22, w: 0.037, y: 0.014 + 0.0007 * i, lift: 0.008, curl: -0.03, part: 2, tone: rnd(), seg: 5, shape: 1 });
  }
  for (i = 0; i < 11; i++) crowsFeather(A, { x: 0.19, z: 0.12 + 0.036 * i, ang: PI - 0.1 - 0.12 * i / 10, len: 0.14, w: 0.03, y: 0.022 + 0.0005 * i, lift: 0.004, curl: 0, part: 2, tone: rnd(), seg: 4, shape: 1 });
  crowsFeather(A, { x: 0.14, z: 0.1, ang: 1.43, len: 0.46, w: 0.055, y: 0.03, lift: 0.01, curl: -0.03, part: 2, tone: rnd(), seg: 6, shape: 1 });
}

// Body and head as one loft whose upper stations follow the head (weight 1), then the beak, the tail
// fan with its coverts, and two legs.
function crowsBody(A, rnd) {
  var PI = Math.PI, i;
  crowsLoft(A, [[-0.34, 0, 0.022, 0.02, 0], [-0.27, 0, 0.056, 0.052, 0], [-0.18, 0.004, 0.09, 0.085, 0], [-0.06, 0.01, 0.12, 0.112, 0], [0.05, 0.012, 0.126, 0.116, 0],
    [0.14, 0.018, 0.112, 0.104, 0.15], [0.21, 0.025, 0.08, 0.078, 0.55], [0.255, 0.03, 0.065, 0.068, 0.9], [0.295, 0.035, 0.072, 0.076, 1], [0.335, 0.035, 0.068, 0.072, 1],
    [0.372, 0.03, 0.048, 0.055, 1], [0.395, 0.022, 0.026, 0.032, 1]], 12, 1, 0.5);
  crowsLoft(A, [[0.36, 0.012, 0.046, 0.042, 1], [0.42, 0.01, 0.04, 0.036, 1], [0.48, 0.0, 0.027, 0.025, 1], [0.535, -0.012, 0.004, 0.004, 1]], 8, 5, 0.5);
  for (i = 0; i < 9; i++) {
    var c = (i - 4) / 4;
    crowsFeather(A, { x: -0.28, z: c * 0.07, ang: PI + c * 0.3, len: 0.4 - 0.07 * Math.abs(c), w: 0.034, y: -0.012 + 0.0006 * (4 - Math.abs(i - 4)), lift: -0.014, curl: -c * 0.05, part: 3, tone: rnd(), seg: 8 });
  }
  for (i = 0; i < 5; i++) { var cc = (i - 2) / 2; crowsFeather(A, { x: -0.2, z: cc * 0.05, ang: PI + cc * 0.25, len: 0.15, w: 0.03, y: 0.012, lift: -0.004, curl: 0, part: 3, tone: rnd(), seg: 4, shape: 1 }); }
  for (var sg = -1; sg <= 1; sg += 2) {
    var an = [-0.02, -0.075, sg * 0.07], kn = [0.03, -0.15, sg * 0.075], ft = [0.0, -0.235, sg * 0.08];
    crowsBar(A, an, kn, 0.016, 0.012, sg); crowsBar(A, kn, ft, 0.012, 0.009, sg);
    [-0.4, 0, 0.4].forEach(function (a) { crowsBar(A, ft, [ft[0] + Math.cos(a) * 0.08, ft[1] - 0.004, ft[2] + Math.sin(a) * 0.08 * sg], 0.009, 0.005, sg); });
    crowsBar(A, ft, [ft[0] - 0.05, ft[1] - 0.004, ft[2]], 0.009, 0.005, sg);
  }
}
// Smooth vertex normals from the triangles.
function crowsNormals(A) {
  var n = new Float32Array(A.pos.length), p = A.pos, i;
  for (i = 0; i < A.idx.length; i += 3) {
    var a = A.idx[i] * 3, b = A.idx[i + 1] * 3, c = A.idx[i + 2] * 3;
    var ux = p[b] - p[a], uy = p[b + 1] - p[a + 1], uz = p[b + 2] - p[a + 2], vx = p[c] - p[a], vy = p[c + 1] - p[a + 1], vz = p[c + 2] - p[a + 2];
    var nx = uy * vz - uz * vy, ny = uz * vx - ux * vz, nz = ux * vy - uy * vx;
    [a, b, c].forEach(function (q) { n[q] += nx; n[q + 1] += ny; n[q + 2] += nz; });
  }
  for (i = 0; i < n.length; i += 3) { var l = Math.sqrt(n[i] * n[i] + n[i + 1] * n[i + 1] + n[i + 2] * n[i + 2]) || 1; n[i] /= l; n[i + 1] /= l; n[i + 2] /= l; }
  return n;
}
// The crow once, as plain arrays: the body parts, the right wing, and the right wing again for the left
// (same bind pose, reversed winding, because the shader mirrors it).
var CROWS_MESH = null;
function crowsMesh() {
  if (CROWS_MESH) return CROWS_MESH;
  var rnd = mulberry(7731), W = crowsAcc(), B = crowsAcc(), i;
  crowsWing(W, rnd); crowsBody(B, rnd);
  var nW = crowsNormals(W), nB = crowsNormals(B), nv = B.n + 2 * W.n;
  var pos = new Float32Array(nv * 3), nrm = new Float32Array(nv * 3), fu = new Float32Array(nv * 2), bind = new Float32Array(nv * 4), idx = new Uint16Array(B.idx.length + 2 * W.idx.length);
  pos.set(B.pos, 0); nrm.set(nB, 0); fu.set(B.fu, 0); bind.set(B.bind, 0);
  for (var s = 0; s < 2; s++) {
    var o = B.n + s * W.n;
    pos.set(W.pos, o * 3); nrm.set(nW, o * 3); fu.set(W.fu, o * 2); bind.set(W.bind, o * 4);
    for (i = 0; i < W.n; i++) bind[(o + i) * 4 + 1] = s ? -1 : 1;
  }
  idx.set(B.idx, 0);
  var k = B.idx.length;
  for (i = 0; i < W.idx.length; i++) idx[k + i] = W.idx[i] + B.n;
  k += W.idx.length;
  for (i = 0; i < W.idx.length; i += 3) { idx[k + i] = W.idx[i] + B.n + W.n; idx[k + i + 1] = W.idx[i + 2] + B.n + W.n; idx[k + i + 2] = W.idx[i + 1] + B.n + W.n; }
  CROWS_MESH = { pos: pos, nrm: nrm, fu: fu, bind: bind, idx: idx };
  return CROWS_MESH;
}

// ---------- Shading ----------
// Per-instance attributes. aFlap: shoulder angle, tip lag, inner sweep, outer sweep. aPose: head pitch,
// tail spread, tail pitch (up positive), legs out (0 tucked, 1 down). aMisc: tone seed, wing twist,
// chord scale (a folded wing is narrower), visibility (below 1 the bird dissolves).
var CROWS_VS = [
  'attribute vec4 aBind; attribute vec4 aFlap; attribute vec4 aPose; attribute vec4 aMisc; attribute vec2 aFu;',
  'varying vec4 vBind; varying vec2 vFu; varying float vVis;',
  // The wing's spine at r (0 shoulder, 1 tip): heading d, the chord c (forward across the wing) and n (up).
  'void crFrame(float r, out vec3 d, out vec3 c, out vec3 n){',
  '  float al = aFlap.x + aFlap.y * r * sqrt(r);',
  '  float be = aFlap.z * smoothstep(0.0, 0.55, r) + aFlap.w * smoothstep(0.4, 1.0, r);',
  '  float ca = cos(al), sa = sin(al), cb = cos(be), sb = sin(be);',
  '  d = vec3(-sb * ca, sa, cb * ca);',
  '  vec3 c0 = vec3(cb, 0.0, sb); vec3 n0 = cross(d, c0);',
  '  float tw = aMisc.y * r, ct = cos(tw), sw = sin(tw);',
  '  c = c0 * ct + n0 * sw; n = n0 * ct - c0 * sw;',
  '}',
  // A wing vertex sits at its spanwise place r on the spine, offset across by its chord and up by its height.
  'void crWing(inout vec3 p, inout vec3 nr){',
  '  float r = clamp((p.z - 0.09) / 0.93, 0.0, 1.0), over = p.z - (0.09 + 0.93 * r);',
  '  vec3 P = vec3(0.12, 0.0, 0.09); float dr = r / 7.0;',
  '  for (int i = 0; i < 7; i++) { vec3 d, c, n; crFrame((float(i) + 0.5) * dr, d, c, n); P += d * dr * 0.93; }',
  '  vec3 d, c, n; crFrame(r, d, c, n);',
  '  vec3 w = P + c * ((p.x - 0.12) * aMisc.z) + n * p.y + d * over;',
  '  vec3 wn = c * nr.x + n * nr.y + d * nr.z;',
  '  w.z *= aBind.y; wn.z *= aBind.y; p = w; nr = wn;',
  '}',
  'void crHead(inout vec3 p, inout vec3 nr, float wt){',
  '  float a = aPose.x * wt, ca = cos(a), sa = sin(a); vec3 q = p - vec3(0.2, 0.02, 0.0);',
  '  p = vec3(0.2, 0.02, 0.0) + vec3(q.x * ca - q.y * sa, q.x * sa + q.y * ca, q.z);',
  '  nr = vec3(nr.x * ca - nr.y * sa, nr.x * sa + nr.y * ca, nr.z);',
  '}',
  'void crTail(inout vec3 p, inout vec3 nr){',
  '  vec3 q = p - vec3(-0.27, 0.0, 0.0); q.z *= aPose.y;',
  '  float a = -aPose.z, ca = cos(a), sa = sin(a);',
  '  p = vec3(-0.27, 0.0, 0.0) + vec3(q.x * ca - q.y * sa, q.x * sa + q.y * ca, q.z);',
  '  nr = vec3(nr.x * ca - nr.y * sa, nr.x * sa + nr.y * ca, nr.z);',
  '}',
  'void crLeg(inout vec3 p){ vec3 an = vec3(-0.02, -0.075, sign(p.z) * 0.07); p = an + (p - an) * mix(0.03, 1.0, aPose.w); }',
  'void crowsDeform(inout vec3 p, inout vec3 nr){',
  '  float pt = aBind.x;',
  '  if (pt > 1.5 && pt < 2.5) crWing(p, nr);',
  '  else if (pt > 2.5 && pt < 3.5) crTail(p, nr);',
  '  else if (pt > 3.5 && pt < 4.5) crLeg(p);',
  '  else if (pt > 0.5) crHead(p, nr, pt > 4.5 ? 1.0 : aBind.z);',
  '}'].join('\n');
var CROWS_HASH = 'float crHash(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }';
var CROWS_FS_PARS = ['uniform vec3 uInk, uSheenA, uSheenB, uBeak; uniform float uSheen;', 'varying vec4 vBind; varying vec2 vFu; varying float vVis;', CROWS_HASH].join('\n');

// Plumage: black with a lighter quill and fine barbs, darker at the root where the next layer covers it,
// and a blue-to-violet sheen that comes up at grazing angles. Glossy rather than wet: roughness stays
// high enough that the key light makes a soft glint on a tilted wing, never a hard white streak.
function crowsLit(sh, U, feather) {
  Object.assign(sh.uniforms, U);
  if (feather) {
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nattribute vec4 aMisc; attribute vec2 aFu;\nvarying vec4 vBind; varying vec2 vFu; varying float vVis;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvBind = vec4(2.0, 1.0, 0.0, aMisc.x); vFu = aFu; vVis = aMisc.w;');
  } else {
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\n' + CROWS_VS)
      .replace('#include <beginnormal_vertex>', 'vec3 crP = position; vec3 crN = normal; crowsDeform(crP, crN);\n#include <beginnormal_vertex>\nobjectNormal = crN;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\ntransformed = crP; vBind = aBind; vFu = aFu; vVis = aMisc.w;');
  }
  sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\n' + CROWS_FS_PARS)
    .replace('#include <clipping_planes_fragment>', '#include <clipping_planes_fragment>\n' + (feather ? '' : 'if (vVis < 0.999 && vVis < crHash(gl_FragCoord.xy)) discard;'))
    .replace('#include <color_fragment>', [
      '#include <color_fragment>',
      feather ? 'diffuseColor.a *= vVis;' : '',
      'float cPart = vBind.x, cTone = vBind.w, fu = vFu.x, fv = vFu.y;',
      'float isF = step(1.5, cPart) * step(cPart, 3.5);',
      'float shaft = exp(-pow((fv - 0.5) * 15.0, 2.0)) * (1.0 - 0.75 * fu);',
      'float barb = 0.5 + 0.5 * sin((fu * 17.0 - abs(fv - 0.5) * 5.5) * 6.2832);',
      'vec3 col = uInk * (0.7 + 0.65 * cTone) * (0.5 + 0.5 * smoothstep(0.0, 0.3, fu * isF + (1.0 - isF))) * (0.92 + 0.08 * barb);',
      'col += vec3(0.05, 0.05, 0.075) * shaft * isF;',
      'if (cPart > 4.5) col = uBeak; else if (cPart > 3.5 && cPart < 4.5) col = vec3(0.075, 0.07, 0.085);',
      'diffuseColor.rgb = col;'].join('\n'))
    .replace('#include <roughnessmap_fragment>', '#include <roughnessmap_fragment>\nroughnessFactor = cPart > 4.5 ? 0.24 : (cPart > 3.5 && cPart < 4.5 ? 0.5 : 0.33 + 0.1 * barb);')
    .replace('#include <emissivemap_fragment>', [
      '#include <emissivemap_fragment>',
      'float cNV = clamp(dot(normal, normalize(vViewPosition)), 0.0, 1.0);',
      'vec3 cS = mix(uSheenA, uSheenB, 0.5 + 0.5 * sin(fu * 5.0 + cTone * 12.0 + fv * 2.0));',
      'totalEmissiveRadiance += cS * (pow(1.0 - cNV, 2.0) * 0.8 + 0.05 + 0.03 * barb * isF) * uSheen * (cPart > 4.5 ? 0.35 : 1.0);'].join('\n'));
}
function crowsUniforms(T) {
  return { uInk: { value: new T.Color(CROWS.ink) }, uSheenA: { value: new T.Color(CROWS.sheenA) }, uSheenB: { value: new T.Color(CROWS.sheenB) }, uBeak: { value: new T.Color(CROWS.beak) }, uSheen: { value: 0.34 } };
}
function crowsMaterial(T) {
  var U = crowsUniforms(T), m = new T.MeshStandardMaterial({ color: 0xffffff, roughness: 0.36, metalness: 0.1, side: T.DoubleSide, envMapIntensity: 0.85 });
  m.onBeforeCompile = function (sh) { crowsLit(sh, U, false); };
  m.userData.U = U;
  return m;
}
// The shadow pass bends the same bird, so the flock's shadows beat their wings too.
function crowsDepth(T) {
  var m = new T.MeshDepthMaterial({ depthPacking: T.RGBADepthPacking, side: T.DoubleSide });
  m.onBeforeCompile = function (sh) {
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\n' + CROWS_VS)
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvec3 crP = position; vec3 crN = vec3(0.0, 1.0, 0.0); crowsDeform(crP, crN); transformed = crP; vVis = aMisc.w;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nvarying float vVis;\n' + CROWS_HASH)
      .replace('#include <clipping_planes_fragment>', '#include <clipping_planes_fragment>\nif (vVis < 0.999 && vVis < crHash(gl_FragCoord.xy)) discard;');
  };
  return m;
}

// ---------- The flock as one draw ----------
// A bird's pose, written into the instance buffers. Angles in radians: yaw turns it about the vertical,
// pitch lifts its nose, roll lowers one wing. F: shoulder flap (up positive); B: tip lag; S1 and S2:
// how far the arm and the hand sweep back; head: head pitch (down negative); tsp, tpi: tail spread and
// pitch; leg: legs out; tw: wing twist; cs: wing chord scale; vis: 1 solid, down to 0 dissolved.
function crowsPoseNew() { return { x: 0, y: 0, z: 0, yaw: 0, pitch: 0, roll: 0, sc: 1, F: 0.1, B: 0.1, S1: 0, S2: 0, head: 0, tsp: 1, tpi: 0, leg: 0, tw: 0, cs: 1, vis: 1, tone: 0 }; }
function crowsFlock(T, n) {
  var M = crowsMesh(), geo = new T.BufferGeometry(), A = {}, i;
  geo.setAttribute('position', new T.BufferAttribute(M.pos.slice(), 3)); geo.setAttribute('normal', new T.BufferAttribute(M.nrm.slice(), 3));
  geo.setAttribute('aFu', new T.BufferAttribute(M.fu.slice(), 2)); geo.setAttribute('aBind', new T.BufferAttribute(M.bind.slice(), 4)); geo.setIndex(new T.BufferAttribute(M.idx.slice(), 1));
  ['aFlap', 'aPose', 'aMisc'].forEach(function (k) { A[k] = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4); A[k].setUsage(T.DynamicDrawUsage); geo.setAttribute(k, A[k]); });
  var mat = crowsMaterial(T), mesh = new T.InstancedMesh(geo, mat, n), depth = crowsDepth(T);
  mesh.instanceMatrix.setUsage(T.DynamicDrawUsage); mesh.frustumCulled = false; mesh.castShadow = true; mesh.customDepthMaterial = depth;
  var m4 = new T.Matrix4(), q = new T.Quaternion(), eu = new T.Euler(0, 0, 0, 'YZX'), pv = new T.Vector3(), sv = new T.Vector3();
  for (i = 0; i < n; i++) { m4.makeScale(0, 0, 0); mesh.setMatrixAt(i, m4); A.aMisc.array[i * 4 + 3] = 0; }
  return { mesh: mesh, mat: mat, depth: depth, n: n,
    set: function (i, b) {
      eu.set(b.roll, b.yaw, b.pitch, 'YZX'); q.setFromEuler(eu); pv.set(b.x, b.y, b.z); sv.set(b.sc, b.sc, b.sc);
      mesh.setMatrixAt(i, m4.compose(pv, q, sv));
      var f = A.aFlap.array, p = A.aPose.array, m = A.aMisc.array, o = i * 4;
      f[o] = b.F; f[o + 1] = b.B; f[o + 2] = b.S1; f[o + 3] = b.S2;
      p[o] = b.head; p[o + 1] = b.tsp; p[o + 2] = b.tpi; p[o + 3] = b.leg;
      m[o] = b.tone; m[o + 1] = b.tw; m[o + 2] = b.cs; m[o + 3] = b.vis;
    },
    hide: function (i) { m4.makeScale(0, 0, 0); mesh.setMatrixAt(i, m4); A.aMisc.array[i * 4 + 3] = 0; },
    commit: function () { mesh.instanceMatrix.needsUpdate = true; A.aFlap.needsUpdate = A.aPose.needsUpdate = A.aMisc.needsUpdate = true; },
    dispose: function () { mesh.dispose(); depth.dispose(); } };
}

// ---------- Wingbeat ----------
// One bird's wings at wingbeat phase ph (cycles), stroke size amp (radians) and rate f (Hz); glide
// 0..1 holds the wings out nearly still. The tip lag is the stroke's own speed, so the primaries trail
// the downstroke upward and the upstroke downward; on the upstroke the hand also folds back a little.
function crowsBeat(P, ph, amp, glide, f) {
  var w = ph * 6.2832, sw = Math.sin(w), cw = Math.cos(w), w2 = 2 * w + 0.6, a = amp * (1 - 0.8 * glide), up = Math.max(0, cw) * (1 - glide);
  P.F = 0.16 + a * (sw + 0.2 * Math.sin(w2));
  P.B = Math.max(-0.85, Math.min(0.85, -0.17 * f * a * (cw + 0.4 * Math.cos(w2)) + 0.14 * glide));
  P.S1 = 0.05 + 0.12 * up; P.S2 = 0.1 + 0.42 * up;
  P.tw = 0.2 * sw * (1 - glide); P.cs = 1;
  P.tsp = 1 + 0.08 * sw; P.tpi = 0.1 * Math.sin(w + 1.3) * (1 - glide); P.head = 0; P.leg = 0;
}

// ---------- Feathers that fall ----------
// One feather mesh, one draw: a lanceolate vane lying along x with a curved quill, tumbling down on a
// flutter (it slips along its length as it rocks, which is how a real feather drifts) and then lying
// on the page. Fading is alpha, so a feather that settles can dim away rather than blink out.
function crowsFeathers(T, n, size, shadow, clear) {
  var A = crowsAcc(), i;
  crowsFeather(A, { x: -0.5, z: 0, ang: 0, len: 1, w: 0.1, y: 0, lift: 0.0, curl: 0.5, part: 2, tone: 0.5, seg: 10 });
  var nm = crowsNormals(A), geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.BufferAttribute(new Float32Array(A.pos), 3)); geo.setAttribute('normal', new T.BufferAttribute(nm, 3));
  geo.setAttribute('aFu', new T.BufferAttribute(new Float32Array(A.fu), 2)); geo.setIndex(new T.BufferAttribute(new Uint16Array(A.idx), 1));
  var mi = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4); mi.setUsage(T.DynamicDrawUsage); geo.setAttribute('aMisc', mi);
  var U = crowsUniforms(T), mat = new T.MeshStandardMaterial({ color: 0xffffff, roughness: 0.4, metalness: 0.1, side: T.DoubleSide, transparent: true, envMapIntensity: 1.1 });
  mat.onBeforeCompile = function (sh) { crowsLit(sh, U, true); };
  var mesh = new T.InstancedMesh(geo, mat, n), m4 = new T.Matrix4(), q = new T.Quaternion(), eu = new T.Euler(0, 0, 0, 'YZX'), pv = new T.Vector3(), sv = new T.Vector3();
  mesh.instanceMatrix.setUsage(T.DynamicDrawUsage); mesh.frustumCulled = false; mesh.castShadow = !!shadow;
  var X = new Float32Array(n), Y = new Float32Array(n), Z = new Float32Array(n), VX = new Float32Array(n), VY = new Float32Array(n), VZ = new Float32Array(n);
  var YAW = new Float32Array(n), YV = new Float32Array(n), PH = new Float32Array(n), PV = new Float32Array(n), SZ = new Float32Array(n), AGE = new Float32Array(n), ST = new Uint8Array(n), SEED = new Float32Array(n), LAND = new Float32Array(n);
  var next = 0;
  for (i = 0; i < n; i++) { m4.makeScale(0, 0, 0); mesh.setMatrixAt(i, m4); }
  return { mesh: mesh,
    // A feather leaves at (x, y, z) with velocity v; sz scales it (1 is the usual length).
    emit: function (x, y, z, vx, vy, vz, sz, rnd) {
      var k = -1, j;
      for (j = 0; j < n; j++) { var c = (next + j) % n; if (!ST[c]) { k = c; break; } }
      if (k < 0) k = next; next = (k + 1) % n;
      X[k] = x; Y[k] = y; Z[k] = z; VX[k] = vx; VY[k] = vy; VZ[k] = vz; YAW[k] = rnd() * 6.283; YV[k] = (rnd() - 0.5) * 5; PH[k] = rnd() * 6.283; PV[k] = 4.5 + rnd() * 3;
      SZ[k] = size * sz * (0.8 + rnd() * 0.4); AGE[k] = 0; ST[k] = 1; SEED[k] = rnd(); LAND[k] = 0;
    },
    step: function (dt, fade) {
      var k, j, any = false, dk = Math.exp(-dt * 2.4);
      for (k = 0; k < n; k++) {
        if (!ST[k]) continue;
        any = true; AGE[k] += dt;
        if (ST[k] === 1) {
          // Air takes the throw, then the feather sinks at its own pace, slipping along its length as it rocks.
          VX[k] *= dk; VZ[k] *= dk; VY[k] = VY[k] * dk + -(1.9 + 0.9 * SEED[k]) * (1 - dk);
          // Blown off any die that has to stay in view.
          for (j = 0; clear && j < clear.length; j++) {
            var qc = clear[j], qx = X[k] - qc.x, qz = Z[k] - qc.z, qR = qc.r * 1.9, qd = Math.sqrt(qx * qx + qz * qz) || 1e-3;
            if (qd < qR) { var qw = 1 - qd / qR; qw *= qw; VX[k] += qx / qd * qw * 40 * dt; VZ[k] += qz / qd * qw * 40 * dt; }
          }
          var slip = Math.sin(PH[k]) * (2.4 + 1.4 * SEED[k]), cy = Math.cos(YAW[k]), sy = Math.sin(YAW[k]);
          X[k] += (VX[k] + cy * slip) * dt; Z[k] += (VZ[k] - sy * slip) * dt; Y[k] += VY[k] * dt;
          PH[k] += PV[k] * dt; YAW[k] += YV[k] * dt * (0.4 + 0.6 * Math.abs(Math.cos(PH[k])));
          if (Y[k] <= 0.03) { Y[k] = 0.03 + 0.0015 * (k % 7); ST[k] = 2; LAND[k] = AGE[k]; }
        }
        var roll = ST[k] === 1 ? 1.15 * Math.sin(PH[k]) : 0.0, pit = ST[k] === 1 ? 0.5 * Math.cos(PH[k] * 0.7) : 0.0;
        if (ST[k] === 2) { var lk = Math.max(0, 1 - (AGE[k] - LAND[k]) / 0.18); roll = 1.15 * Math.sin(PH[k]) * lk; pit = 0.5 * Math.cos(PH[k] * 0.7) * lk; }
        eu.set(roll, YAW[k], pit, 'YZX'); q.setFromEuler(eu); pv.set(X[k], Y[k], Z[k]);
        var a = Math.min(1, AGE[k] / 0.1) * fade, sc = SZ[k] * (a > 0.02 ? 1 : 0);
        // A feather over a die to keep in view shrinks to nothing.
        for (j = 0; clear && j < clear.length; j++) sc *= smooth(clear[j].r * 0.95, clear[j].r * 1.4, Math.hypot(X[k] - clear[j].x, Z[k] - clear[j].z));
        sv.set(sc, sc, sc);
        mesh.setMatrixAt(k, m4.compose(pv, q, sv)); mesh.geometry.attributes.aMisc.array[k * 4] = SEED[k]; mesh.geometry.attributes.aMisc.array[k * 4 + 3] = a;
      }
      mesh.instanceMatrix.needsUpdate = true; mi.needsUpdate = true;
      return any;
    },
    dispose: function () { mesh.dispose(); } };
}

// ---------- The page under the flock ----------
// A soft violet-black patch that darkens the page as the flock passes, with a hole round every place
// that has to stay in view (up to 8 other crit dice).
function crowsShade(T, at, R, clear) {
  var holes = [], k;
  for (k = 0; k < 8; k++) holes.push(clear && clear[k] ? new T.Vector3(clear[k].x, clear[k].z, clear[k].r) : new T.Vector3(0, 0, 0));
  var U = { uOp: { value: 0 }, uPh: { value: 0 }, uC: { value: new T.Vector2(at.x, at.z) }, uR: { value: R }, uHole: { value: holes }, uCol: { value: new T.Color(CROWS.shade) } };
  var mat = new T.ShaderMaterial({ uniforms: U, transparent: true, depthWrite: false,
    vertexShader: 'varying vec2 vW; void main(){ vW = (modelMatrix * vec4(position, 1.0)).xz; gl_Position = projectionMatrix * viewMatrix * modelMatrix * vec4(position, 1.0); }',
    fragmentShader: ['uniform float uOp, uR, uPh; uniform vec2 uC; uniform vec3 uHole[8], uCol; varying vec2 vW;',
      'void main(){',
      // Spiral arms that turn with the flock, so the dark over the page swirls rather than sits.
      ' vec2 d = (vW - uC) / uR; float sw = 0.5 + 0.5 * sin(atan(d.y, d.x) * 3.0 + log(length(d) + 0.15) * 4.0 - uPh);',
      ' float a = uOp * exp(-dot(d, d) * 1.5) * (0.8 + 0.3 * sw);',
      ' for (int i = 0; i < 8; i++) { float r = uHole[i].z; if (r > 0.0) a *= mix(0.3, 1.0, smoothstep(r * 0.6, r * 1.3, length(vW - uHole[i].xy))); }',
      ' gl_FragColor = vec4(uCol, a); }'].join('\n') });
  var m = new T.Mesh(new T.PlaneGeometry(R * 3, R * 3), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.004, at.z); m.renderOrder = -2; m.frustumCulled = false;
  return m;
}
// The crows' own shadows, a second layer over the stage's faint one: only the shadow map shows, so
// the flock's shadows are dark and sweep over the page with it.
function crowsShadowFloor(st) {
  var T = st.T, m = new T.Mesh(new T.PlaneGeometry(st.hw * 3, st.hh * 3), new T.ShadowMaterial({ opacity: 0 }));
  m.rotation.x = -Math.PI / 2; m.position.y = 0.006; m.receiveShadow = true; m.renderOrder = -1; m.frustumCulled = false;
  return m;
}
// A flat ring of dark (and, to show on a dark page, a trace of light) that runs out over the page. Its
// band is thinner than the shared wave texture's, so a small ring reads as a ring and not as a plate.
var CROWS_RINGTEX = null;
function crowsRingTex(T) {
  if (CROWS_RINGTEX) return CROWS_RINGTEX;
  var c = canvas(256, 256), g = c.getContext('2d'), gr = g.createRadialGradient(128, 128, 0, 128, 128, 128);
  gr.addColorStop(0, 'rgba(255,255,255,0)'); gr.addColorStop(0.76, 'rgba(255,255,255,0)'); gr.addColorStop(0.9, 'rgba(255,255,255,0.6)');
  gr.addColorStop(0.96, 'rgba(255,255,255,1)'); gr.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = gr; g.fillRect(0, 0, 256, 256);
  CROWS_RINGTEX = new T.CanvasTexture(c); CROWS_RINGTEX.colorSpace = T.SRGBColorSpace; share(CROWS_RINGTEX); return CROWS_RINGTEX;
}
function crowsRing(st, at, color, from, to, dur, dark, order) {
  var T = st.T, m = new T.Mesh(new T.PlaneGeometry(2, 2), new T.MeshBasicMaterial({ map: crowsRingTex(T), color: color, transparent: true, opacity: 0, depthWrite: false, side: T.DoubleSide,
    blending: dark ? T.NormalBlending : T.AdditiveBlending, toneMapped: false }));
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.03, at.z); m.renderOrder = order || 0; m.frustumCulled = false; st.scene.add(m);
  var t = 0;
  return { update: function (dt) {
    t += dt; var k = Math.min(1, t / dur), r = from + (to - from) * out3(k);
    m.scale.set(r, r, r); m.material.opacity = (dark ? 0.42 : 0.26) * (1 - k) * (1 - k);
    if (k >= 1) { st.scene.remove(m); m.geometry.dispose(); m.material.dispose(); return false; }
    return true;
  } };
}

// The 1 drawn lit in a colour, over a wash of that colour across its face: the face goes violet-red
// even where there is no bloom and no extra light.
function crowsNumeral(gf, color, wash) {
  var g = gf.g, L = gf.lay, gr;
  gf.clear();
  if (wash > 0) {
    gr = g.createRadialGradient(L.cx, L.cy, 0, L.cx, L.cy, L.r * 1.25);
    gr.addColorStop(0, hexA(color, 0.6 * wash)); gr.addColorStop(1, hexA(color, 0.25 * wash));
    g.fillStyle = gr; g.fillRect(0, 0, gf.cv, gf.cv);
  }
  numberOnto(g, L, color);
}

// ---------- Shared flight helpers ----------
function crowsWrap(a) { while (a > Math.PI) a -= 6.2832; while (a < -Math.PI) a += 6.2832; return a; }
// Keeps a bird (p.x, p.z) out of every place to keep clear: never nearer its centre than the place's radius
// plus m (about a wing and a half), sliding round it as a flock splits round a post. The push is continuous
// everywhere; a bird that would fly straight through the centre slips by on the side facing (gx, gz).
function crowsAvoid(clear, m, p, gx, gz) {
  for (var k = 0; k < clear.length; k++) {
    var c = clear[k], R = c.r + m, dx = p.x - c.x, dz = p.z - c.z, d = Math.sqrt(dx * dx + dz * dz), t = d / R;
    if (t >= 1.35) continue;
    var bx = gx - c.x, bz = gz - c.z, bl = Math.sqrt(bx * bx + bz * bz) || 1, w = Math.exp(-4 * t * t);
    var ux = dx / R + bx / bl * w, uz = dz / R + bz / bl * w, ul = Math.sqrt(ux * ux + uz * uz) || 1, dd = R * (1 + 0.35 * Math.pow(t / 1.35, 3.86));
    p.x = c.x + ux / ul * dd; p.z = c.z + uz / ul * dd;
  }
}
// Heading, bank and climb for a bird that has moved by (vx, vy, vz) per second, eased so a turn is a
// curve and the wing on the inside of it drops.
function crowsHeading(P, b, vx, vy, vz, dt) {
  var sp = Math.sqrt(vx * vx + vz * vz);
  if (sp > 0.4) {
    var ty = Math.atan2(-vz, vx), dy = crowsWrap(ty - b.yaw) * (1 - Math.exp(-dt * 16));
    b.yawRate += (dy / Math.max(dt, 1e-3) - b.yawRate) * (1 - Math.exp(-dt * 12)); b.yaw += dy;
  }
  b.roll += (Math.max(-0.85, Math.min(0.85, -0.28 * b.yawRate)) - b.roll) * (1 - Math.exp(-dt * 10));
  b.pitch += (Math.max(-0.9, Math.min(0.9, Math.atan2(vy, sp + 0.5) * 0.9)) - b.pitch) * (1 - Math.exp(-dt * 10));
  P.yaw = b.yaw; P.roll = b.roll; P.pitch = b.pitch;
}

var CROWS_LIVE = null;   // the grand's flock while it plays: an echo takes a bird from it
var CROWS_BEAT = 2.1, CROWS_BURST = 3.35, CROWS_END = 4.75;

// ---------- The grand ----------
// The sequence, on t: the flock streams in over the page from one edge (0 to 1.5), spirals tighter round
// the die (1.2 to 3.3) while the 1 bruises; from 1.7 five birds in turn drop out of the circle, flare,
// peck at the die and climb back (the die jolts at each peck); at 3.35 the whole flock bursts outward
// off the page, throwing feathers; the last feathers settle and fade by 4.75.
function fxCrows(st, die, fi, th, o) {
  var T = st.T, hd = st.q.detail !== 'basic', s = die.size, g = die.group, rnd = mulberry(8123);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), cx = p0.x, cz = p0.z, rest = die.rest, t = 0, done = false, burst = false;
  var N = hd ? 18 : 9, ND = hd ? 5 : 3, U = s * 1.08, clear = (o && o.clear) || [], i;
  // Glass has nothing dark behind its number, so the same light and bloom would blot it out: less of each.
  var gk = die.back ? 0.5 : 1, KEEP = 1.4 * U;
  var flock = crowsFlock(T, N), feathers = crowsFeathers(T, hd ? 56 : 28, s * 0.62, hd, clear);
  var shade = crowsShade(T, p0, s * 7, clear), floor = crowsShadowFloor(st);
  var light = hd ? new T.PointLight(CROWS.bruise, 0, s * 9, 1.5) : null, gf = glowFace(die, fi), fc = glowLayer(die, fi);
  st.scene.add(floor, shade, flock.mesh, feathers.mesh); if (light) st.scene.add(light);

  // The orbit's winding angle over time, tabulated: the spin quickens as the circle tightens.
  var STEP = 1 / 120, TH = [0], k;
  function omega(tt) { return 0.95 + 1.9 * smooth(0.3, 2.0, tt) + 1.5 * smooth(2.0, 3.2, tt); }
  for (k = 1; k <= Math.ceil(CROWS_END / STEP) + 2; k++) TH.push(TH[k - 1] + omega((k - 0.5) * STEP) * STEP);
  function theta(tt) { var f = Math.max(0, tt) / STEP, j = Math.min(TH.length - 2, Math.floor(f)); return TH[j] + (TH[j + 1] - TH[j]) * (f - j); }

  // They come in from the edge farther from the die, bending toward the middle of the page. Every bird
  // follows the same spiral a moment after the one before it, in one of three lanes (their own radius
  // and height), so the flock is a stream that winds up round the die and no two birds ever meet.
  var fromLeft = cx > 0, thE = fromLeft ? Math.PI : 0, edgeX = (fromLeft ? -1 : 1) * (st.hw + 1.6 * U), dir = -(cz >= 0 ? 1 : -1) * (fromLeft ? -1 : 1);
  var birds = [], P = crowsPoseNew(), pickD = [], perLane = N / 3;
  for (i = 0; i < N; i++) {
    var lane = i % 3, kk = Math.floor(i / 3);
    birds.push({ lane: lane, a0: (rnd() - 0.5) * 0.12, roff: (lane - 1) * s, dly: lane * 0.08 + kk * 1.3 / perLane + rnd() * 0.03, R0: Math.abs(edgeX - cx) + rnd() * 0.3 * s,
      Rc: 5.0 * s, Rt: 3.5 * s, hy: (1.2 + 0.8 * lane) * s, freq: 3.9 + rnd(), amp: 0.62 + rnd() * 0.2, ph: rnd(), wph: rnd() * 6.28, gph: rnd() * 6.28, tone: rnd(),
      x: 0, y: 0, z: 0, vx: 0, vy: 0, vz: 0, yaw: 0, roll: 0, pitch: 0, yawRate: 0, ox: 0, oy: 0, oz: 0, an: 0, R: 0, w: 0, diver: null, gone: false, taken: false, out: 0, up: 0, bfreq: 6 });
  }
  // The divers: spread through the flock, each with its own moment.
  for (i = 0; i < ND; i++) {
    var b = birds[Math.floor((i + 0.5) * N / ND)];
    b.diver = { tD: 1.7 + 0.3 * i, tP: 2.1 + 0.3 * i, qx: 0, qz: 0, qy: rest + 0.3 * s, hit: false, dx: 0, dz: 0 }; pickD.push(b);
  }
  function orbit(b, tt) {
    var ti = Math.max(0, tt - b.dly), e = 1 - Math.pow(1 - clamp01(ti / 1.55), 2);
    b.R = b.R0 + (b.Rc - b.R0) * e + (b.Rt - b.Rc) * smooth(1.2, 3.2, ti) + b.roff + 0.12 * s * Math.sin(2.3 * ti + b.wph) * smooth(0.8, 1.6, ti);
    b.an = thE + b.a0 + dir * theta(ti);
    b.ox = cx + Math.cos(b.an) * b.R; b.oz = cz + Math.sin(b.an) * b.R;
    b.oy = b.hy + 0.9 * s * (1 - e) + 0.15 * s * Math.sin(1.9 * ti + b.wph) - 0.25 * s * smooth(2.0, 3.2, ti);
  }
  // Where each diver will be when it pecks: just off the die, on the side it is circling past.
  pickD.forEach(function (b) {
    orbit(b, b.diver.tP); var d = 0.9 * s + 0.55 * U;
    b.diver.qx = cx + Math.cos(b.an) * d; b.diver.qz = cz + Math.sin(b.an) * d;
    var l = Math.hypot(b.diver.qx - cx, b.diver.qz - cz) || 1; b.diver.dx = (cx - b.diver.qx) / l; b.diver.dz = (cz - b.diver.qz) / l;
  });
  function place(b, tt) {
    orbit(b, tt);
    var d = b.diver, w = 0;
    if (d) w = smooth(d.tD, d.tP, tt) * (1 - smooth(d.tP + 0.14, d.tP + 0.6, tt));
    b.w = w;
    b.x = b.ox + ((d ? d.qx : 0) - b.ox) * w; b.z = b.oz + ((d ? d.qz : 0) - b.oz) * w; b.y = b.oy + ((d ? d.qy : 0) - b.oy) * w;
    crowsAvoid(clear, KEEP, b, cx, cz);
    // Squeezed in toward the die by a place to keep clear, a bird climbs to fly over it.
    var dg = Math.hypot(b.x - cx, b.z - cz), top = (rest + 1.5 * s) * (1 - smooth(1.8 * s, 3.4 * s, dg));
    if (top > b.y) b.y += (top - b.y) * (1 - w);
  }
  // Start with the velocity the first frame would see.
  birds.forEach(function (b) { place(b, -1 / 60); b.px = b.x; b.py = b.y; b.pz = b.z; place(b, 0); b.vx = (b.x - b.px) * 60; b.vy = (b.y - b.py) * 60; b.vz = (b.z - b.pz) * 60; b.yaw = Math.atan2(-b.vz, b.vx); });

  // What an echo may peel a bird from: where each bird is and how it is flying, and which are taken.
  var LV = CROWS_LIVE = { die: die, N: N, scale: U, x: new Float32Array(N), y: new Float32Array(N), z: new Float32Array(N), vx: new Float32Array(N), vy: new Float32Array(N), vz: new Float32Array(N),
    ph: new Float32Array(N), yaw: new Float32Array(N), f: new Float32Array(N), taken: new Uint8Array(N), gone: new Uint8Array(N), diver: new Uint8Array(N) };
  birds.forEach(function (b, q) { LV.diver[q] = b.diver ? 1 : 0; });

  // The die: a damped spring for each knock, so it shudders and settles where it lay.
  var jx = 0, jz = 0, jvx = 0, jvz = 0, ta = 0, tv = 0, tax = 0, taz = 1, flash = 0, lastStage = -1, sheds = 0;
  function knock(dx, dz, k) { jvx += dx * 2.6 * k; jvz += dz * 2.6 * k; tv += 1.7 * k; tax = dz; taz = -dx; flash = Math.min(1.6, flash + k); }
  function springs(dt) {
    var n = Math.ceil(dt / 0.008), h = dt / n;
    for (var q = 0; q < n; q++) {
      jvx += (-700 * jx - 11 * jvx) * h; jvz += (-700 * jz - 11 * jvz) * h; jx += jvx * h; jz += jvz * h;
      tv += (-700 * ta - 11 * tv) * h; ta += tv * h;
    }
  }
  var qT = new T.Quaternion(), axT = new T.Vector3();
  function shed(b, n, out, up) {
    for (var q = 0; q < n; q++) {
      var a = rnd() * 6.283, sp = out * (0.5 + rnd());
      feathers.emit(b.x, b.y, b.z, b.vx * 0.25 + Math.cos(a) * sp, up * (0.4 + rnd()), b.vz * 0.25 + Math.sin(a) * sp, 0.8 + rnd() * 0.5, rnd);
    }
  }
  function visibleBird() { for (var q = 0; q < 8; q++) { var b = birds[Math.floor(rnd() * N)]; if (!b.gone && !b.taken) return b; } return null; }

  // The bursting flock is simulated: every bird gets thrown outward and upward from where it was.
  // Each bird flies off along the heading nearest straight out that misses every place to keep clear: the
  // throw is run forward for each candidate and the first that stays clear of them all is taken.
  function burstStep(q, hx, hz, out, h) {
    q.vx += hx * out * h; q.vz += hz * out * h; q.vx *= 1 - 0.25 * h; q.vz *= 1 - 0.25 * h; q.x += q.vx * h; q.z += q.vz * h;
  }
  function burstClear(b, ang, out) {
    var q = { x: b.x, z: b.z, vx: b.vx, vz: b.vz }, hx = Math.cos(ang), hz = Math.sin(ang), room = 1e9, k, d, n;
    for (n = 0; n < 70; n++) {
      burstStep(q, hx, hz, out, 0.02);
      for (k = 0; k < clear.length; k++) { d = Math.hypot(q.x - clear[k].x, q.z - clear[k].z) - clear[k].r - KEEP; if (d < room) room = d; }
      if (Math.abs(q.x) > st.hw + 3 * U || Math.abs(q.z) > st.hh + 3 * U) break;
    }
    return room;
  }
  function launch() {
    burst = true;
    birds.forEach(function (b) {
      b.out = 36 + rnd() * 18; b.up = 5 + rnd() * 5; b.bfreq = 5.8 + rnd() * 1.4;
      var base = Math.atan2(b.z - cz, b.x - cx), best = base, bestRoom = -1e9, j, off, a, r;
      for (j = 0; j < 25 && bestRoom < 0; j++) {
        off = Math.ceil(j / 2) * 0.2618 * (j % 2 ? 1 : -1); a = base + off; r = clear.length ? burstClear(b, a, b.out) : 1;
        if (r > bestRoom) { bestRoom = r; best = a; }
      }
      b.hx = Math.cos(best); b.hz = Math.sin(best);
    });
    // The rings stop short of the nearest die to keep in view.
    var c = { x: cx, z: cz }, reach = s * 4.6;
    clear.forEach(function (q) { reach = Math.min(reach, Math.hypot(q.x - cx, q.z - cz) - q.r); });
    reach = Math.max(reach, s * 2.4);
    st.fx.push(crowsRing(st, c, '#0b0712', s * 1.3, reach, 0.7, true, -1));
    st.fx.push(crowsRing(st, c, CROWS.bruise, s * 1.1, reach * 0.85, 0.8, false, 0));
    st.fx.push(spray(st, { n: hd ? 22 : 10, colors: ['#1c1626', '#2a2034'], size: s * 1.5, life: [0.9, 1.7], drag: 1.7, g: -0.2, blend: 'normal', smoke: true, alpha: 0.5,
      spawn: function (q, r) { var a = r() * 6.28, sp = (1 + r() * 3) * s; return { p: [cx, rest * 0.5, cz], v: [Math.cos(a) * sp, 0.3 + r() * 0.8, Math.sin(a) * sp] }; } }));
    for (var q = 0; q < (hd ? 16 : 8); q++) { var b2 = birds[q % N]; if (!b2.gone && !b2.taken) shed(b2, 1, 4, 3); }
    knock(0.4, -0.2, 0.5); flash = 1.6;
  }

  function frame(dt) {
    var i2, b, d, w;
    if (t >= CROWS_BURST && !burst) launch();
    var live = CROWS_LIVE;
    for (i2 = 0; i2 < N; i2++) {
      b = birds[i2]; d = b.diver;
      if (live && live.taken[i2]) b.taken = true;
      if (b.taken) { flock.hide(i2); continue; }
      if (b.gone) continue;
      var px = b.x, py = b.y, pz = b.z;
      if (!burst) place(b, t);
      else {
        var sub = Math.ceil(dt / 0.02), h = dt / sub;
        for (var u = 0; u < sub; u++) {
          burstStep(b, b.hx, b.hz, b.out, h); b.vy += b.up * h; b.y += b.vy * h; crowsAvoid(clear, U * 0.8, b, cx, cz);
        }
      }
      if (dt > 0) { b.vx = (b.x - px) / dt; b.vy = (b.y - py) / dt; b.vz = (b.z - pz) / dt; }
      // Out beyond the page edge: gone.
      if (burst && (Math.abs(b.x) > st.hw + 3 * U || Math.abs(b.z) > st.hh + 3 * U) && t > CROWS_BURST + 0.15) { b.gone = true; if (live) live.gone[i2] = 1; flock.hide(i2); continue; }
      crowsHeading(P, b, b.vx, b.vy, b.vz, dt);
      var glide = burst ? 0 : 0.55 * smooth(0.55, 0.95, 0.5 + 0.5 * Math.sin(t * 1.7 + b.gph)) * smooth(1.0, 1.6, t), amp = b.amp, f = b.freq, flare = 0, stoop = 0, pk = 0;
      if (burst) { amp = 0.95; f = b.bfreq; }
      if (d) {
        stoop = smooth(d.tD, d.tD + 0.15, t) * (1 - smooth(d.tP - 0.12, d.tP - 0.02, t));
        flare = smooth(d.tP - 0.12, d.tP, t) * (1 - smooth(d.tP + 0.25, d.tP + 0.6, t));
        var hard = Math.max(flare, smooth(d.tP + 0.1, d.tP + 0.2, t) * (1 - smooth(d.tP + 0.5, d.tP + 0.9, t)));
        amp += (1.0 - amp) * hard; f += (7 - f) * hard; glide = Math.max(glide * (1 - hard), stoop * 0.9);
        // The face turns to the die as it comes in.
        var fy = Math.atan2(-(cz - b.z), cx - b.x), face = smooth(0.55, 1.0, b.w);
        P.yaw = b.yaw + crowsWrap(fy - b.yaw) * face;
        var dtp = t - d.tP; pk = Math.exp(-Math.pow((dtp - 0.02) / 0.1, 2));
      }
      b.ph += f * dt;
      crowsBeat(P, b.ph, amp, glide, f);
      if (d) {
        P.F += (-0.15 - P.F) * stoop; P.S1 += (0.65 - P.S1) * stoop; P.S2 += (1.3 - P.S2) * stoop; P.tsp = P.tsp * (1 - stoop) + 0.5 * stoop + 0.55 * flare;
        P.tpi -= 0.55 * flare; P.leg = 0.8 * flare; P.head = -1.05 * pk; P.pitch += 0.5 * flare;
        // The peck: the die is knocked away from the bird, once.
        if (!d.hit && t >= d.tP + 0.03) {
          d.hit = true; knock(d.dx, d.dz, 1); shed(b, hd ? 5 : 3, 2.2, 3.5);
        }
      }
      P.x = b.x; P.y = b.y; P.z = b.z; P.sc = U; P.tone = b.tone; P.vis = 1;
      flock.set(i2, P);
      if (live) { live.x[i2] = b.x; live.y[i2] = b.y; live.z[i2] = b.z; live.vx[i2] = b.vx; live.vy[i2] = b.vy; live.vz[i2] = b.vz; live.ph[i2] = b.ph; live.yaw[i2] = b.yaw; live.f[i2] = f; }
    }
    flock.commit();
  }

  return { update: function (dt) {
    if (done) return false;
    t += dt;
    // Quiet shedding while they circle.
    if (t > 0.9 && t < CROWS_BURST) { sheds += (hd ? 12 : 6) * dt; while (sheds >= 1) { sheds -= 1; var sb = visibleBird(); if (sb && sb.R < 9 * s) shed(sb, 1, 1.2, 0.6); } }
    frame(dt);
    var ramp = smooth(0.15, 1.3, t) * (1 - smooth(CROWS_BURST + 0.55, CROWS_BURST + 1.25, t)), fade = 1 - smooth(CROWS_END - 0.75, CROWS_END, t);
    feathers.step(dt, fade);
    shade.material.uniforms.uOp.value = 0.28 * ramp; shade.material.uniforms.uPh.value = 1.8 * dir * theta(t); floor.material.opacity = 0.3 * ramp;

    // The die: bruises over the circling, flashes at each peck, shudders on its spot.
    flash *= Math.exp(-dt * 7);
    var bru = 0.65 * smooth(0.9, 3.3, t) + 0.35 * smooth(3.3, 3.9, t), stage = Math.round(bru * 6);
    if (stage !== lastStage) { lastStage = stage; crowsNumeral(gf, mix('#7c40d8', CROWS.bruise, stage / 6), die.back ? 0 : stage / 6); gf.commit(); }
    fc.mat.emissiveIntensity = (0.3 + 0.8 * bru + 0.6 * flash) * gk * smooth(0.2, 0.9, t);
    tintDie(die, CROWS.deep, (0.3 * bru + 0.18 * flash) * gk * smooth(0.5, 1.2, t));
    if (light) { light.position.set(cx + 1.3 * s, rest + 1.3 * s, cz); light.intensity = 15 * gk * gk * bru * (0.7 + 0.6 * flash); }
    springs(dt);
    g.position.set(cx + jx, p0.y + Math.abs(ta) * 0.4, cz + jz);
    g.quaternion.copy(q0).premultiply(qT.setFromAxisAngle(axT.set(tax, 0, taz).normalize(), ta));

    if (t >= CROWS_END) {
      done = true; g.position.copy(p0); g.quaternion.copy(q0); tintDie(die, CROWS.deep, 0);
      crowsNumeral(gf, CROWS.bruise, 0); gf.commit(die.back ? 0.9 : 1.3);
      drop(st, [flock.mesh, feathers.mesh, shade, floor, light]); flock.dispose(); feathers.dispose();
      if (CROWS_LIVE && CROWS_LIVE.die === die) CROWS_LIVE = null;
      return false;
    }
    return true;
  } };
}

// ---------- The echo ----------
// One crow: it peels off the grand's flock (or comes in from the nearest page edge when there is no
// flock), flares onto the top of the die with its wings folding, dips its head in a caw that sends a
// dark ring over the page, crouches and flies off the page. Under 2 s.
function fxCrowsEcho(st, die, fi, th, e) {
  var T = st.T, s = die.size, g = die.group, hd = st.q.detail !== 'basic', rnd = mulberry(5511 + ((e && e.order) || 0) * 17), t = 0, done = false, k = -1, i;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), cx = p0.x, cz = p0.z, fc = glowLayer(die, fi), gf = glowFace(die, fi), top = worldOf(die, die.faces[fi].c);
  var live = CROWS_LIVE && e && e.from && CROWS_LIVE.die === e.from ? CROWS_LIVE : null, U = live ? live.scale : s * 1.08;
  if (live) {
    for (i = 0; i < live.N; i++) { var c = ((e.order || 0) * 3 + 1 + i) % live.N; if (!live.taken[c] && !live.diver[c] && !live.gone[c]) { k = c; break; } }
    if (k >= 0) live.taken[k] = 1; else live = null;
  }
  var flock = crowsFlock(T, 1), feathers = crowsFeathers(T, 6, s * 0.62, hd), floor = crowsShadowFloor(st), P = crowsPoseNew();
  floor.material.opacity = 0.22; st.scene.add(floor, flock.mesh, feathers.mesh);

  // Where it starts and how it is moving: the grand's bird, or the nearest page edge.
  var S = new T.Vector3(), V0 = new T.Vector3(), edgeR = st.hw + 2.4 * U - cx, edgeL = st.hw + 2.4 * U + cx, edgeD = st.hh + 2.4 * U - cz, edgeU = st.hh + 2.4 * U + cz, ph0 = rnd(), f0 = 4.4;
  if (live) { S.set(live.x[k], live.y[k], live.z[k]); V0.set(live.vx[k], live.vy[k], live.vz[k]); ph0 = live.ph[k]; f0 = live.f[k] || 4.4; }
  else {
    var m = Math.min(edgeR, edgeL, edgeD, edgeU), lat = (rnd() - 0.5) * 3 * s;
    if (m === edgeL) S.set(-st.hw - 2.4 * U, 2.6 * s, cz + lat); else if (m === edgeR) S.set(st.hw + 2.4 * U, 2.6 * s, cz + lat);
    else if (m === edgeU) S.set(cx + lat, 2.6 * s, -st.hh - 2.4 * U); else S.set(cx + lat, 2.6 * s, st.hh + 2.4 * U);
  }
  var Ax = cx - S.x, Az = cz - S.z, Al = Math.hypot(Ax, Az) || 1; Ax /= Al; Az /= Al;
  if (!live) V0.set(Ax * 26, -2, Az * 26);
  // It lands on the far side of the top face, tail toward the middle, so the number is mostly clear.
  var Pf = new T.Vector3(top.x + Ax * 0.42 * s, top.y + 0.245 * U, top.z + Az * 0.42 * s), hl = Math.atan2(-Az, Ax), Vf = new T.Vector3(Ax * 2.5, -1.5, Az * 2.5);
  var dist = S.distanceTo(Pf), T1 = clamp01((dist / (live ? 14 : 26) - 0.4) / 0.22) * 0.22 + 0.4, Tc = T1 + 0.18, Tl = T1 + 0.5, END = Tl + 0.8;
  // A bird already circling fast must not overshoot on its way over: its start speed is capped to what the trip can use.
  var m0 = T1 * V0.length(); if (m0 > 0.9 * dist) V0.multiplyScalar(0.9 * dist / m0);
  // Off the page by the nearest edge that is not behind it.
  var ex = [[1, 0, edgeR], [-1, 0, edgeL], [0, 1, edgeD], [0, -1, edgeU]], bx = Ax, bz = Az, bd = 1e9;
  ex.forEach(function (d) { if (d[0] * Ax + d[1] * Az > -0.17 && d[2] < bd) { bd = d[2]; bx = d[0]; bz = d[1]; } });

  var b = { x: S.x, y: S.y, z: S.z, vx: V0.x, vy: V0.y, vz: V0.z, yaw: Math.atan2(-V0.z, V0.x), roll: 0, pitch: 0, yawRate: 0, ph: ph0 }, cawed = false, landed = false, left = false, exv = 0, exu = 0;

  function setNum(k2) { fc.mat.emissiveIntensity = k2; }
  return { update: function (dt) {
    if (done) return false;
    t += dt;
    var px = b.x, py = b.y, pz = b.z, flare = 0, fold = 0, leg = 0, head = 0, amp = 1.0, f = 6.6, vis = 1;
    if (t < T1) {
      var u = t / T1, u2 = u * u, u3 = u2 * u, h00 = 2 * u3 - 3 * u2 + 1, h10 = u3 - 2 * u2 + u, h01 = -2 * u3 + 3 * u2, h11 = u3 - u2;
      b.x = h00 * S.x + h10 * T1 * V0.x + h01 * Pf.x + h11 * T1 * Vf.x; b.y = h00 * S.y + h10 * T1 * V0.y + h01 * Pf.y + h11 * T1 * Vf.y; b.z = h00 * S.z + h10 * T1 * V0.z + h01 * Pf.z + h11 * T1 * Vf.z;
      b.vx = (b.x - px) / dt; b.vy = (b.y - py) / dt; b.vz = (b.z - pz) / dt;
      crowsHeading(P, b, b.vx, b.vy, b.vz, dt);
      flare = smooth(T1 - 0.22, T1 - 0.02, t); leg = smooth(T1 - 0.26, T1 - 0.06, t); fold = smooth(T1 - 0.05, T1 + 0.14, t);
      f = f0 + (7.2 - f0) * flare; amp = 0.75 + 0.3 * flare;
    } else if (t < Tl) {
      b.x = Pf.x; b.y = Pf.y; b.z = Pf.z; b.vx = b.vy = b.vz = 0; b.roll *= Math.exp(-dt * 14); b.pitch *= Math.exp(-dt * 14); b.yaw += crowsWrap(hl - b.yaw) * (1 - Math.exp(-dt * 14));
      P.yaw = b.yaw; P.roll = b.roll; P.pitch = b.pitch; fold = 1; leg = 1; flare = Math.max(0, 1 - (t - T1) / 0.14) * 0.6;
      // The caw: head down and out, then back.
      head = -1.0 * Math.exp(-Math.pow((t - Tc - 0.09) / 0.09, 2)) - 0.18 * Math.exp(-Math.pow((t - Tc - 0.3) / 0.07, 2));
    } else {
      var tau = t - Tl, crouch = smooth(0, 0.07, tau) * (1 - smooth(0.07, 0.12, tau)), go = smooth(0.06, 0.14, tau);
      exv = go * (4 + 40 * Math.max(0, tau - 0.08)); exu = go * (3 + 8 * Math.max(0, tau - 0.08));
      b.vx = bx * exv; b.vz = bz * exv; b.vy = exu;
      b.x += b.vx * dt; b.z += b.vz * dt; b.y += b.vy * dt * (tau > 0.08 ? 1 : 0);
      if (go > 0.02) crowsHeading(P, b, b.vx, b.vy, b.vz, dt); else { P.yaw = b.yaw; P.roll = b.roll; P.pitch = b.pitch; }
      P.pitch -= 0.3 * crouch; fold = 1 - smooth(0.08, 0.2, tau); leg = 1 - smooth(0.1, 0.35, tau); head = 0.3 * crouch;
      vis = 1 - smooth(0.45, 0.78, tau);
      if (!left) { left = true; for (i = 0; i < 2; i++) feathers.emit(b.x, b.y, b.z, (rnd() - 0.5) * 2, 2 + rnd() * 2, (rnd() - 0.5) * 2, 0.9 + rnd() * 0.4, rnd); }
    }
    b.ph += f * dt;
    crowsBeat(P, b.ph, amp, 0, f);
    // Folding: the arm and hand sweep back along the body and the wing narrows.
    P.F += (0.05 - P.F) * fold; P.B += (0.1 - P.B) * fold; P.S1 += (0.95 - P.S1) * fold; P.S2 += (0.6 - P.S2) * fold; P.cs = 1 - 0.2 * fold; P.tw *= 1 - fold;
    P.tsp = 1 + 0.5 * flare + 0.3 * fold * 0; P.tpi = -0.5 * flare - 0.12 * fold; P.leg = leg; P.head = head; P.pitch += 0.6 * flare * (t < T1 ? 1 : 0);
    P.x = b.x; P.y = b.y; P.z = b.z; P.sc = U; P.tone = 0.3; P.vis = vis;
    flock.set(0, P); flock.commit();

    if (!landed && t >= T1) { landed = true; feathers.emit(b.x, b.y, b.z, (rnd() - 0.5), 1.5, (rnd() - 0.5), 1, rnd); }
    if (!cawed && t >= Tc + 0.05) {
      cawed = true; var at = { x: cx, z: cz };
      st.fx.push(crowsRing(st, at, '#0b0712', s * 1.3, s * 2.7, 0.5, true, 61)); st.fx.push(crowsRing(st, at, CROWS.bruise, s * 1.2, s * 2.5, 0.6, false, 60));
      gf.clear(); numberOnto(gf.g, gf.lay, CROWS.bruise); gf.commit();
    }
    feathers.step(dt, 1 - smooth(END - 0.3, END, t));
    // The die dips a hair as it takes the bird's weight, and the 1 lights at the caw.
    var dip = t >= T1 && t < Tl ? 0.035 * s * Math.min(1, (t - T1) / 0.05) * Math.exp(-Math.max(0, t - T1 - 0.05) * 4) : 0;
    g.position.set(cx, p0.y - dip, cz);
    if (cawed) setNum((0.85 * smooth(Tc + 0.05, Tc + 0.35, t) + 0.9 * Math.exp(-(t - Tc - 0.05) * 6)) * (die.back ? 0.6 : 1));
    if (t >= END) {
      done = true; g.position.copy(p0); g.quaternion.copy(q0); setNum(die.back ? 0.6 : 0.9);
      drop(st, [flock.mesh, feathers.mesh, floor]); flock.dispose(); feathers.dispose();
      return false;
    }
    return true;
  } };
}
registerCrit('crows', { name: 'Murder of crows', kind: 'fail', what: 'Crows sweep across the page and mob the die.', fx: fxCrows, beat: CROWS_BEAT, peak: CROWS_BURST + 0.2,
  echo: fxCrowsEcho, echoWhat: 'One crow lands on the die, caws and flies off.' });
