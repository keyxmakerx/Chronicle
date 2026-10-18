// ---------- Crit fail: The abyss ----------
// The page under the die cracks, then the ground falls away into a deep pit: rings of rock ledges step
// down toward a black void with a faint cold glow far below, and crumbling debris tumbles from ledge to
// ledge and shrinks into the dark. The die tilts and sinks halfway (the page is a clipping plane, so
// what is below it is not drawn), hangs, claws its way back up in lurches, and settles on the page as
// the pit closes, leaving a faint crater scar that fades and the 1 lit a cold deep blue.
// The pit and the cracks are drawn in the opaque pass (cracks first, pit over them) so they never need
// to know where each other's edge is, and the pit can sit below the floor plane's depth.
import {
  canvas, clamp01, drop, env4, glow, glowFace, mix, mulberry, numberOnto, out3, registerCrit,
  shadeDie, smooth, spriteTex, starTex, strokeW, tintDie
} from '../engine.js';

var ABYSS_GLOW = '#4d86ff', ABYSS_ICE = '#b9d0ff', ABYSS_DEEP = '#2456e0', ABYSS_INK = '#05060e';

// Noise for the pit rock and the page layers. Prefixed so it never collides with another effect's copy.
var ABYSS_GLSL = [
  '#ifndef AB_OCT', '#define AB_OCT 3', '#endif',
  'float abH(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float abH3(vec3 p){ return fract(sin(dot(p, vec3(127.1, 311.7, 74.7))) * 43758.5453); }',
  'float abN(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(abH(i), abH(i + vec2(1.0, 0.0)), u.x), mix(abH(i + vec2(0.0, 1.0)), abH(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float abN3(vec3 p){ vec3 i = floor(p), f = fract(p); vec3 u = f * f * (3.0 - 2.0 * f);',
  ' float a = abH3(i), b = abH3(i + vec3(1.0, 0.0, 0.0)), c = abH3(i + vec3(0.0, 1.0, 0.0)), d = abH3(i + vec3(1.0, 1.0, 0.0));',
  ' float e = abH3(i + vec3(0.0, 0.0, 1.0)), f1 = abH3(i + vec3(1.0, 0.0, 1.0)), g = abH3(i + vec3(0.0, 1.0, 1.0)), h = abH3(i + vec3(1.0, 1.0, 1.0));',
  ' return mix(mix(mix(a, b, u.x), mix(c, d, u.x), u.y), mix(mix(e, f1, u.x), mix(g, h, u.x), u.y), u.z); }',
  'float abF3(vec3 p){ float s = 0.0, a = 0.5; for (int i = 0; i < AB_OCT; i++) { s += a * abN3(p); p = p * 2.07 + 1.3; a *= 0.5; } return s; }'].join('\n');

// A pale stone needs the number in a deeper blue: pale blue light on ivory has nothing to stand on.
function abyssLightStone(th) {
  var h = th && th.base && th.base.grad && th.base.grad[0];
  if (!h || h.charAt(0) !== '#') return false;
  var n = parseInt(h.slice(1), 16);
  return (0.3 * (n >> 16 & 255) + 0.59 * (n >> 8 & 255) + 0.11 * (n & 255)) / 255 > 0.62;
}
// The face's number as cold light; on a pale stone it is a solid deep blue with a halo instead.
function abyssNumeral(die, fi, th) {
  var gf = glowFace(die, fi), pale = abyssLightStone(th), key = '';
  return { pale: pale, set: function (color, k) {
    if (color !== key) {
      key = color; gf.clear();
      if (pale && gf.lay.style !== 'classic') {
        var g2 = gf.g, w = strokeW(gf.lay);
        g2.lineJoin = 'round'; g2.lineCap = 'round'; g2.shadowColor = color; g2.shadowBlur = w; g2.strokeStyle = color; g2.lineWidth = w * 1.25;
        gf.lay.strokes.forEach(function (pl) { g2.beginPath(); g2.moveTo(pl[0][0], pl[0][1]); for (var q = 1; q < pl.length; q++) g2.lineTo(pl[q][0], pl[q][1]); g2.stroke(); });
        g2.shadowBlur = 0;
      } else numberOnto(gf.g, gf.lay, color);
    }
    gf.commit(k);
  } };
}
// The die's circumradius in world units.
function abyssRadius(die) {
  var r = 0; die.sd.v.forEach(function (v) { r = Math.max(r, Math.sqrt(v.x * v.x + v.y * v.y + v.z * v.z)); });
  return r * die.size;
}
// A smooth wrap-around signal in -1..1 from a few low sine harmonics: ledges and edges that wander
// without a seam where the angle wraps.
function abyssHarm(NA, rnd, m0, m1) {
  var out = new Float32Array(NA), ph = [], am = [], tot = 0, m, j;
  for (m = m0; m <= m1; m++) { ph.push(rnd() * 6.2832); am.push(1 / Math.pow(m, 0.8)); tot += am[am.length - 1]; }
  for (j = 0; j < NA; j++) { var a = j / NA * 6.2832, v = 0; for (m = m0; m <= m1; m++) v += am[m - m0] * Math.sin(m * a + ph[m - m0]); out[j] = v / tot * 1.7; }
  return out;
}

// ---------- The pit ----------
// A surface of revolution, as rings of vertices: the torn page edge, then rock ledges stepping down, each
// a flat shelf and a steep wall, narrowing to a point far below. Radii are fractions of the mouth radius;
// depths are in die sizes. [outer radius, inner radius, depth at the outer edge].
var ABYSS_LEDGES = [[0.92, 0.77, 0.60], [0.665, 0.55, 1.55], [0.465, 0.375, 2.70], [0.30, 0.23, 4.10], [0.17, 0.12, 5.90], [0.06, 0.0, 8.20]];
var ABYSS_DEPTH = 8.2;
// kind: 0 rock, 1 the soot fringe round the mouth, 2 the cut edge of the page. grp indexes the jitter
// tables (0 is the rim, then one per ledge) and f blends toward the next group, so a wall stays between
// its two ledges however they wander.
function abyssProfile() {
  var P = [{ u: 1.075, y: 0.014, ao: 1, kind: 1, t: 1, ga: 0, gb: 0, f: 0, amp: 0.03 }, { u: 1.0, y: 0.014, ao: 1, kind: 1, t: 0, ga: 0, gb: 0, f: 0, amp: 0.03 },
    { u: 0.99, y: -0.02, ao: 1, kind: 2, t: 0, ga: 0, gb: 0, f: 0, amp: 0.03 }];
  var pu = 0.99, py = -0.02, pg = 0;
  ABYSS_LEDGES.forEach(function (L, k) {
    for (var m = 1; m <= 2; m++) {
      var f = m / 3;
      P.push({ u: pu + (L[0] - pu) * f, y: py + (-L[2] - py) * f, ao: m === 1 ? 0.85 : 0.7, kind: k === 0 ? 2 : 0, t: 0, ga: pg, gb: k + 1, f: f, amp: 0.04, wall: 1 });
    }
    if (k === 0) P.push({ u: L[0] + 0.003, y: -L[2] + 0.012, ao: 0.5, kind: 2, t: 0, ga: 1, gb: 1, f: 0, amp: 0.04 });
    P.push({ u: L[0], y: -L[2], ao: 0.42, kind: 0, t: 0, ga: k + 1, gb: k + 1, f: 0, amp: 0.045, led: k });
    P.push({ u: L[1], y: -L[2] - 0.07, ao: 1, kind: 0, t: 0, ga: k + 1, gb: k + 1, f: 0, amp: 0.045, led: k });
    pu = L[1]; py = -L[2] - 0.07; pg = k + 1;
  });
  return P;
}

// The pit mesh. open (0..1) is how far the mouth has opened, raggedly, sector by sector; deep (0..1) how
// far the floor has fallen. lim holds the largest mouth radius at each angle, so a ring of rock never
// reaches a die that must stay in view. heightAt reads the current surface for the falling debris.
// Each band between two rings has its own vertices and one normal across it, so a shelf and the wall
// under it shade as two clean faces instead of blurring into each other.
function abyssPit(st, hd, at, s, lim, rnd, U) {
  var T = st.T, NA = lim.length, NC = NA + 1, prof = abyssProfile(), NR = prof.length, NB = NR - 1, NV = NB * 2 * NC, NL = ABYSS_LEDGES.length;
  var rp = new Float32Array(NR * NC * 3), pos = new Float32Array(NV * 3), nor = new Float32Array(NV * 3), aAO = new Float32Array(NV), aKind = new Float32Array(NV);
  var ringR = new Float32Array(NR * NA), ringY = new Float32Array(NR * NA), cs = new Float32Array(NA), sn = new Float32Array(NA);
  var crumble = abyssHarm(NA, rnd, 3, 9), hj = [], hy = [], jit = new Float32Array(NR * NA), jy = new Float32Array(NR * NA), i, j, k;
  for (j = 0; j < NA; j++) { cs[j] = Math.cos(j / NA * 6.2832); sn[j] = Math.sin(j / NA * 6.2832); crumble[j] = clamp01(crumble[j] * 0.5 + 0.5); }
  for (k = 0; k <= NL; k++) { hj.push(abyssHarm(NA, rnd, 2, 7)); hy.push(abyssHarm(NA, rnd, 3, 9)); }
  for (i = 0; i < NR; i++) {
    var pr = prof[i];
    for (j = 0; j < NA; j++) {
      var wob = pr.amp * ((1 - pr.f) * hj[pr.ga][j] + pr.f * hj[pr.gb][j]);
      jit[i * NA + j] = wob + (pr.wall ? (rnd() - 0.5) * 0.016 : 0);
      jy[i * NA + j] = pr.kind === 1 || pr.u > 0.98 ? 0 : 0.13 * ((1 - pr.f) * hy[pr.ga][j] + pr.f * hy[pr.gb][j]) + (pr.wall ? (rnd() - 0.5) * 0.12 : 0);
    }
  }
  var idx = [], bd, sd;
  for (bd = 0; bd < NB; bd++) {
    for (sd = 0; sd < 2; sd++) for (j = 0; j < NC; j++) { var pq = prof[bd + sd], vi = (bd * 2 + sd) * NC + j; aAO[vi] = pq.kind === 1 ? pq.t : pq.ao; aKind[vi] = pq.kind; }
    for (j = 0; j < NA; j++) { var a = bd * 2 * NC + j, b = a + NC; idx.push(a, b, a + 1, a + 1, b, b + 1); }
  }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.BufferAttribute(nor, 3));
  geo.setAttribute('aAO', new T.BufferAttribute(aAO, 1)); geo.setAttribute('aKind', new T.BufferAttribute(aKind, 1)); geo.setIndex(idx);
  var mat = abyssPitMat(st, hd, U), mesh = new T.Mesh(geo, mat);
  mesh.frustumCulled = false; mesh.castShadow = true; mesh.receiveShadow = true; mesh.visible = false;
  mat.shadowSide = T.DoubleSide;

  function update(open, deep) {
    var dp = Math.pow(clamp01(deep), 0.85), o3;
    for (j = 0; j < NA; j++) {
      var m = lim[j] * out3(clamp01(open * 1.5 - 0.5 * crumble[j]));
      for (i = 0; i < NR; i++) {
        var p = prof[i], r = p.u * (1 + jit[i * NA + j]) * m * (p.y < -0.03 ? 1 - 0.2 * dp * clamp01(-p.y / ABYSS_DEPTH) : 1), y = p.y > -0.03 ? p.y : (p.y + jy[i * NA + j]) * s * dp;
        o3 = (i * NC + j) * 3; rp[o3] = at.x + cs[j] * r; rp[o3 + 1] = y; rp[o3 + 2] = at.z + sn[j] * r;
        ringR[i * NA + j] = r; ringY[i * NA + j] = y;
      }
    }
    for (i = 0; i < NR; i++) { var s0 = (i * NC) * 3, s1 = (i * NC + NA) * 3; rp[s1] = rp[s0]; rp[s1 + 1] = rp[s0 + 1]; rp[s1 + 2] = rp[s0 + 2]; }
    // The ring direction (outer to inner) crossed with the angle direction points up on a shelf and in
    // toward the middle on a wall, which is the side the camera sees.
    for (bd = 0; bd < NB; bd++) for (j = 0; j < NC; j++) {
      var jj = j % NA, jm = (jj + NA - 1) % NA, jp = (jj + 1) % NA, A = (bd * NC + jj) * 3, B = ((bd + 1) * NC + jj) * 3;
      var rx = rp[B] - rp[A], ry = rp[B + 1] - rp[A + 1], rz = rp[B + 2] - rp[A + 2];
      var tx = rp[(bd * NC + jp) * 3] + rp[((bd + 1) * NC + jp) * 3] - rp[(bd * NC + jm) * 3] - rp[((bd + 1) * NC + jm) * 3];
      var ty = rp[(bd * NC + jp) * 3 + 1] + rp[((bd + 1) * NC + jp) * 3 + 1] - rp[(bd * NC + jm) * 3 + 1] - rp[((bd + 1) * NC + jm) * 3 + 1];
      var tz = rp[(bd * NC + jp) * 3 + 2] + rp[((bd + 1) * NC + jp) * 3 + 2] - rp[(bd * NC + jm) * 3 + 2] - rp[((bd + 1) * NC + jm) * 3 + 2];
      var nx = ry * tz - rz * ty, ny = rz * tx - rx * tz, nz = rx * ty - ry * tx, nl = Math.sqrt(nx * nx + ny * ny + nz * nz);
      if (nl < 1e-9) { nx = 0; ny = 1; nz = 0; nl = 1; }
      var v0 = (bd * 2 * NC + j) * 3, v1 = v0 + NC * 3;
      pos[v0] = rp[A]; pos[v0 + 1] = rp[A + 1]; pos[v0 + 2] = rp[A + 2]; pos[v1] = rp[B]; pos[v1 + 1] = rp[B + 1]; pos[v1 + 2] = rp[B + 2];
      nor[v0] = nor[v1] = nx / nl; nor[v0 + 1] = nor[v1 + 1] = ny / nl; nor[v0 + 2] = nor[v1 + 2] = nz / nl;
    }
    geo.attributes.position.needsUpdate = true; geo.attributes.normal.needsUpdate = true;
  }
  // The surface height under a point, and whether that point is over the pit at all.
  var hit = { y: 0, inside: false };
  function heightAt(px, pz) {
    var dx = px - at.x, dz = pz - at.z, r = Math.sqrt(dx * dx + dz * dz), an = Math.atan2(dz, dx);
    if (an < 0) an += 6.2832;
    var jj = Math.round(an / 6.2832 * NA) % NA;
    // Rings 0 to 2 are the fringe and the lip: the page, not the pit.
    if (r >= ringR[2 * NA + jj]) { hit.y = 0; hit.inside = false; return hit; }
    hit.inside = true;
    for (var q = 2; q < NR - 1; q++) {
      var ra = ringR[q * NA + jj], rb = ringR[(q + 1) * NA + jj];
      if (r >= rb) { var f = ra - rb > 1e-6 ? (ra - r) / (ra - rb) : 0; hit.y = ringY[q * NA + jj] * (1 - f) + ringY[(q + 1) * NA + jj] * f; return hit; }
    }
    hit.y = ringY[(NR - 1) * NA + jj]; return hit;
  }
  // The lip's radius at an angle, for shedding rock from the edge.
  function rimAt(an) { an = (an % 6.2832 + 6.2832) % 6.2832; return ringR[2 * NA + Math.floor(an / 6.2832 * NA) % NA]; }
  return { mesh: mesh, mat: mat, update: update, heightAt: heightAt, rimAt: rimAt, NA: NA };
}

// The rock: strata, grain and pitting from noise, bumped by the same noise, lit by the room's own sun
// (so the lip shades the ledges and the die shades them too), then darkened by depth and by how tucked
// in a spot is, with a cold glow rising toward the floor. The soot fringe round the mouth is cut ragged
// by noise, so the cracks beneath show through where it crumbles.
function abyssPitMat(st, hd, U) {
  var T = st.T, uni = { uGlow: { value: 0 }, uCold: { value: new T.Color(ABYSS_GLOW) }, uDepthRef: { value: U.depth }, uBump: { value: hd ? 1.0 : 0.7 } };
  var mat = new T.MeshStandardMaterial({ color: '#ffffff', roughness: 0.86, metalness: 0, envMapIntensity: 0.5 });
  mat.defines = { AB_OCT: hd ? 4 : 3 };
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nattribute float aAO; attribute float aKind; varying float vAO; varying float vKind; varying vec3 vWp;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvAO = aAO; vKind = aKind; vWp = (modelMatrix * vec4(transformed, 1.0)).xyz;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', ['#include <common>',
        'uniform float uGlow, uDepthRef, uBump; uniform vec3 uCold; varying float vAO; varying float vKind; varying vec3 vWp;', ABYSS_GLSL,
        'vec3 abPerturb(vec3 pos, vec3 n, vec2 dh, float fd){ vec3 sx = normalize(dFdx(pos)), sy = normalize(dFdy(pos)); vec3 r1 = cross(sy, n), r2 = cross(n, sx);',
        ' float det = dot(sx, r1) * fd; vec3 gr = sign(det) * (dh.x * r1 + dh.y * r2); return normalize(abs(det) * n - gr); }'].join('\n'))
      .replace('#include <clipping_planes_fragment>', ['#include <clipping_planes_fragment>',
        'if (vKind > 0.5 && vKind < 1.2) { float abT = vAO; float abC = abN3(vWp * 5.5) * 0.65 + abH3(floor(vWp * 13.0)) * 0.35; if (abC < abT * 1.15 - 0.04) discard; }'].join('\n'))
      .replace('#include <color_fragment>', ['#include <color_fragment>',
        'vec3 abP = vWp;',
        'float abStrata = abN3(vec3(abP.x * 0.55, abP.y * 2.6, abP.z * 0.55));',
        'float abGrain = abF3(abP * 2.6);',
        'float abPit = abF3(abP * 8.0 + 3.7);',
        'vec3 abRock = mix(vec3(0.08, 0.08, 0.115), vec3(0.19, 0.175, 0.225), abGrain);',
        'abRock = mix(abRock, vec3(0.30, 0.24, 0.21), smoothstep(0.56, 0.82, abStrata) * 0.55);',
        'abRock *= 0.7 + 0.6 * abPit;',
        'float abCut = smoothstep(1.55, 1.95, vKind), abSoot = step(0.5, vKind) * (1.0 - smoothstep(1.15, 1.6, vKind));',
        // The cut edge of the page is paler, layered paper-and-soil, so it shows as a clean edge round the mouth.
        'abRock = mix(abRock, vec3(0.28, 0.27, 0.28) * (0.6 + 0.5 * abGrain) * (0.8 + 0.4 * abStrata), abCut);',
        'abRock = mix(abRock, vec3(0.045, 0.042, 0.055), abSoot);',
        'diffuseColor.rgb = abRock;'].join('\n'))
      .replace('#include <roughnessmap_fragment>', '#include <roughnessmap_fragment>\nroughnessFactor = clamp(0.8 + (abPit - 0.5) * 0.3, 0.4, 1.0);')
      .replace('#include <normal_fragment_maps>', ['#include <normal_fragment_maps>',
        'float abHt = abF3(abP * 3.3) * 0.7 + abF3(abP * 11.0) * 0.3;',
        'normal = abPerturb(-vViewPosition, normal, vec2(dFdx(abHt), dFdy(abHt)) * uBump * 0.9, faceDirection);'].join('\n'))
      .replace('#include <emissivemap_fragment>', ['#include <emissivemap_fragment>',
        'float abDf = clamp(-vWp.y / uDepthRef, 0.0, 1.0);',
        'vec3 abNw = inverseTransformDirection(normal, viewMatrix);',
        // Dust settles on the shelves and not on the walls.
        'diffuseColor.rgb = mix(diffuseColor.rgb, vec3(0.34, 0.33, 0.38) * (0.7 + 0.5 * abGrain), smoothstep(0.75, 1.0, abNw.y) * 0.25 * (1.0 - abCut) * (1.0 - abSoot));',
        'totalEmissiveRadiance += uCold * uGlow * (smoothstep(0.3, 1.0, abDf) * (0.08 + 0.3 * (0.5 + 0.5 * abNw.y)) + pow(abDf, 3.0) * 0.2) * (0.5 + 0.5 * vAO);'].join('\n'))
      .replace('#include <aomap_fragment>', ['float abDk = (0.04 + 0.96 * pow(1.0 - clamp(abDf * 1.6, 0.0, 1.0), 1.7)) * (0.25 + 0.75 * vAO) * 0.5;',
        'reflectedLight.directDiffuse *= abDk; reflectedLight.indirectDiffuse *= abDk; reflectedLight.directSpecular *= abDk; reflectedLight.indirectSpecular *= abDk;',
        '#include <aomap_fragment>'].join('\n'));
  };
  mat.userData.uni = uni;
  return mat;
}

// ---------- Cracks ----------
// Jagged cracks as segments in canvas px round the centre: a few long ones running out from under the
// die, branching at wide angles, tapering toward their tips. d is the distance along the path, which
// becomes the time each bit of crack opens.
function abyssCrackSegs(C, rnd, hd, r0, maxR) {
  var segs = [], step = C / (hd ? 48 : 36), cx = C / 2, wk = C / 1024;
  // lv 0 is a main crack, 1 a branch off it, 2 a twig off a branch.
  function walk(x, y, a, L, w, lv, d) {
    var n = Math.max(2, Math.round(L / step));
    for (var i = 1; i <= n; i++) {
      a += (rnd() - 0.5) * (lv ? 1.1 : 0.95);
      if (lv === 0) { var ra = Math.atan2(y - cx, x - cx), da = ra - a; da = Math.atan2(Math.sin(da), Math.cos(da)); a += da * 0.12; }
      var sl = step * (0.55 + rnd() * 0.9), nx = x + Math.cos(a) * sl, ny = y + Math.sin(a) * sl;
      if ((nx - cx) * (nx - cx) + (ny - cx) * (ny - cx) > maxR * maxR) break;
      d += sl;
      segs.push([x, y, nx, ny, d, Math.max(0.9 * wk, w * (1 - 0.65 * i / n)), lv]);
      x = nx; y = ny;
      if (lv < 2 && i > 1 && i < n - 1 && rnd() < (lv ? 0.07 : 0.15))
        walk(x, y, a + (rnd() < 0.5 ? 1 : -1) * (0.5 + rnd() * 0.75), (n - i) * step * (0.3 + rnd() * 0.45), w * 0.62, lv + 1, d);
    }
  }
  var NS = hd ? 9 : 7;
  for (var k = 0; k < NS; k++) {
    var a0 = k / NS * 6.2832 + (rnd() - 0.5) * 0.5, rr = r0 * (0.9 + rnd() * 0.3);
    walk(cx + Math.cos(a0) * rr, cx + Math.sin(a0) * rr, a0 + (rnd() - 0.5) * 0.3, (maxR - rr) * (0.5 + rnd() * 0.5), 6.4 * wk, 0, 0);
  }
  var dMax = 0; segs.forEach(function (sg) { dMax = Math.max(dMax, sg[4]); });
  return { segs: segs, dMax: dMax };
}
// The cracks painted as data, not colour: red is coverage (a soft halo, then the crack itself), blue the
// time each bit opens, both multiplied by coverage so the mipmaps still average to the right time.
function abyssCrackBatches(cr) {
  var LV = 64, by = {}, out = [], passes = [{ R: 30, wk: 3.6, w0: 2 }, { R: 90, wk: 2.3, w0: 1.2 }, { R: 255, wk: 1, w0: 0 }];
  passes.forEach(function (ps, pi) {
    cr.segs.forEach(function (sg) {
      var b = Math.min(LV - 1, Math.floor(sg[4] / cr.dMax * LV)), wq = sg[5] > 3.4 ? 2 : sg[5] > 1.9 ? 1 : 0, key = pi + '|' + wq + '|' + b, bt = by[key];
      if (!bt) { bt = by[key] = { pass: pi, b: b, style: 'rgb(' + ps.R + ',0,' + Math.round(ps.R * (b + 0.5) / LV) + ')', w: [1.2, 2.2, 3.6][wq] * ps.wk + ps.w0, s: [] }; out.push(bt); }
      bt.s.push(sg);
    });
  });
  out.sort(function (p, q) { return p.pass - q.pass || p.b - q.b; });
  return out;
}
function abyssPaintBatch(g, bt) {
  g.strokeStyle = bt.style; g.lineWidth = bt.w; g.beginPath();
  for (var i = 0; i < bt.s.length; i++) { var sg = bt.s[i]; g.moveTo(sg[0], sg[1]); g.lineTo(sg[2], sg[3]); }
  g.stroke();
}

// A ring that is dark behind its front and has a thin pale crest on it: x is the distance from the front
// in widths, rg is (radius, width, strength), and the result is (dark, crest).
var ABYSS_RING = 'vec2 abRing(float r, vec3 rg){ float x = (r - rg.x) / max(rg.y, 0.001); float tr = smoothstep(-0.75, -0.12, x) * (1.0 - smoothstep(-0.12, 0.2, x)); float cr = exp(-pow((x - 0.05) / 0.16, 2.0)); return vec2(tr, cr) * rg.z; }';

// ---------- The page layer ----------
// One plane on the page, drawn before the pit: a dark soft dim round the die, the cracks (black with
// cold light seeping up out of them), a shock ring, and the crater scar. Premultiplied, so the same
// pixel can both darken the page and add light. Round every die in view it falls away to nothing.
// uLin is 1 when the glow pass blends in linear light, where a dark layer looks much weaker.
function abyssLayerMat(st, tex, at, holes) {
  var T = st.T, hs = [], i;
  for (i = 0; i < 8; i++) hs.push(new T.Vector3(0, 0, 0));
  holes.slice(0, 8).forEach(function (h, k) { hs[k].set(h.x, h.z, h.r); });
  var uni = { uMap: { value: tex }, uGrow: { value: 0 }, uCrackK: { value: 0 }, uGlow: { value: 0 }, uDim: { value: 0 }, uDimR: { value: 5 }, uScar: { value: 0 }, uScarR: { value: 3 }, uTime: { value: 0 },
    uLin: { value: st.cine ? 1 : 0 }, uCenter: { value: new T.Vector2(at.x, at.z) }, uHoles: { value: hs }, uNH: { value: Math.min(8, holes.length) },
    uRingA: { value: new T.Vector3(0, 1, 0) }, uRingB: { value: new T.Vector3(0, 1, 0) }, uCold: { value: new T.Color(ABYSS_GLOW) }, uInk: { value: new T.Color(ABYSS_INK) } };
  return new T.ShaderMaterial({ uniforms: uni, transparent: false, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    vertexShader: 'varying vec2 vUv; varying vec3 vW; void main(){ vUv = uv; vec4 w = modelMatrix * vec4(position, 1.0); vW = w.xyz; gl_Position = projectionMatrix * viewMatrix * w; }',
    fragmentShader: [
      'uniform sampler2D uMap; uniform float uGrow, uCrackK, uGlow, uDim, uDimR, uScar, uScarR, uTime, uLin, uNH; uniform vec2 uCenter; uniform vec3 uHoles[8]; uniform vec3 uRingA, uRingB, uCold, uInk;',
      'varying vec2 vUv; varying vec3 vW;', ABYSS_GLSL,
      ABYSS_RING,
      'void main(){',
      ' vec2 q = vW.xz - uCenter; float r = length(q); vec2 au = q / max(r, 0.0001);',
      ' float keep = 1.0;',
      ' for (int i = 0; i < 8; i++) { if (float(i) >= uNH) break; vec3 h = uHoles[i]; keep *= smoothstep(h.z, h.z * 1.3, length(vW.xz - h.xy)); }',
      ' vec4 tx = texture2D(uMap, vUv); float cov = tx.r; float b = cov > 0.01 ? tx.b / cov : 0.0, ahead = uGrow - b;',
      ' float on = smoothstep(0.0, 0.015, ahead) * step(0.001, uGrow);',
      ' float halo = smoothstep(0.03, 0.11, cov) * on * (0.5 + 0.5 * abN(vW.xz * 7.0)), body = smoothstep(0.22, 0.34, cov) * on, core = smoothstep(0.62, 0.96, cov) * on;',
      ' float tip = exp(-max(ahead, 0.0) * 55.0) * step(ahead, 0.25) * core * (1.0 - smoothstep(0.8, 1.0, uGrow)), prox = 1.0 - b;',
      ' float glow = (core * uGlow * (0.3 + 1.0 * prox * prox) + tip * 1.1) * mix(1.7, 1.0, uLin);',
      ' float dark = (body * 0.92 + halo * 0.3) * uCrackK;',
      ' float dimA = uDim * smoothstep(uDimR, uDimR * 0.2, r);',
      ' float sn = abN(au * 2.5 + 4.0) * 0.5 + abN(vW.xz * 2.0) * 0.5, sR = uScarR * (0.85 + 0.3 * sn);',
      ' float brk = 0.4 + 0.6 * abN(vec2(au.x * 4.0 + r * 1.5, au.y * 4.0 + 1.3)), ring = exp(-pow((r - sR) / (0.13 * uScarR), 2.0)) * brk, disc = smoothstep(sR, sR * 0.6, r);',
      ' float scar = uScar * (ring * 0.34 + disc * 0.05) / 0.6;',
      ' vec2 ra = abRing(r, uRingA), rb = abRing(r, uRingB);',
      ' float dk = clamp(dimA * 0.62 + dark + scar * 0.6 + (ra.x + rb.x) * 0.5, 0.0, 0.96) * keep;',
      ' vec3 gl = mix(uCold, vec3(0.75, 0.88, 1.0), 0.3) * (glow + (ra.y + rb.y) * 0.18) * keep;',
      ' dk = 1.0 - pow(1.0 - dk, mix(1.0, 2.0, uLin));',
      ' gl_FragColor = vec4(uInk * dk + gl, dk);',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
}

// ---------- Debris ----------
// A broken lump of rock: a jittered, flattened icosahedron with sharp facets.
function abyssRockGeo(T) {
  var geo = new T.IcosahedronGeometry(1, 0), P = geo.attributes.position, v = new T.Vector3();
  for (var i = 0; i < P.count; i++) {
    v.fromBufferAttribute(P, i);
    // Jitter keyed on the rounded position, so the copies of a corner in each triangle move together.
    var h = Math.sin(Math.round(v.x * 1000) * 0.01299 + Math.round(v.y * 1000) * 0.07823 + Math.round(v.z * 1000) * 0.03771) * 43758.5453;
    v.multiplyScalar(0.7 + 0.5 * (h - Math.floor(h))); v.y *= 0.6;
    P.setXYZ(i, v.x, v.y, v.z);
  }
  geo.computeVertexNormals();
  return geo;
}
// Lit by the room, then darkened by depth like the pit, so a chunk fades into the dark as it falls.
function abyssRockMat(st, U) {
  var T = st.T, uni = { uDepthRef: { value: U.depth } };
  var mat = new T.MeshStandardMaterial({ color: '#ffffff', roughness: 0.9, metalness: 0, flatShading: true, envMapIntensity: 0.5 });
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nvarying float vAbY;')
      .replace('#include <begin_vertex>', ['#include <begin_vertex>', 'vec4 abW = vec4(transformed, 1.0);', '#ifdef USE_INSTANCING', 'abW = instanceMatrix * abW;', '#endif', 'vAbY = (modelMatrix * abW).y;'].join('\n'));
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nuniform float uDepthRef; varying float vAbY;')
      .replace('#include <aomap_fragment>', ['float abDk = 0.06 + 0.94 * pow(1.0 - clamp(-vAbY / uDepthRef * 1.6, 0.0, 1.0), 1.6);',
        'reflectedLight.directDiffuse *= abDk; reflectedLight.indirectDiffuse *= abDk; reflectedLight.directSpecular *= abDk; reflectedLight.indirectSpecular *= abDk;',
        '#include <aomap_fragment>'].join('\n'));
  };
  return mat;
}
// Chunks fall under gravity, bounce on the pit's current surface, and roll in toward the middle; where a
// shelf ends they drop to the next. They shrink with depth and are gone once they reach the black.
// A chunk that meets a wall from the side turns back instead of climbing it.
function abyssDebris(st, hd, s, at, pit, U) {
  var T = st.T, N = hd ? 72 : 38, im = new T.InstancedMesh(abyssRockGeo(T), abyssRockMat(st, U), N), rnd = mulberry(913), i;
  var ZERO = new T.Matrix4().makeScale(0, 0, 0), m4 = new T.Matrix4(), sc = new T.Vector3(), dq = new T.Quaternion(), col = new T.Color(), cs = [], cur = 0;
  var tones = ['#4a4954', '#5a5762', '#3d3c47', '#8a8794', '#6a6270', '#4f4a58'];
  im.frustumCulled = false; im.castShadow = hd;
  for (i = 0; i < N; i++) {
    im.setMatrixAt(i, ZERO); im.setColorAt(i, col.set(tones[i % tones.length]));
    cs.push({ live: false, p: new T.Vector3(), v: new T.Vector3(), q: new T.Quaternion(), ax: new T.Vector3(), w: 0, sc: 1, ky: 1, kz: 1 });
  }
  im.instanceColor.needsUpdate = true;
  var G = s * 26, depthW = U.depthWorld;
  return { mesh: im,
    // fn(j, r) gives { x, y, z, vx, vy, vz, sc, w } for one chunk.
    emit: function (n, fn) {
      for (var j = 0; j < n; j++) {
        var c = cs[cur], d = fn(j, rnd); cur = (cur + 1) % N;
        c.p.set(d.x, d.y, d.z); c.v.set(d.vx, d.vy, d.vz); c.sc = d.sc; c.w = d.w; c.ky = 0.6 + rnd() * 0.7; c.kz = 0.7 + rnd() * 0.6;
        c.ax.set(rnd() - 0.5, rnd() - 0.5, rnd() - 0.5).normalize(); c.q.setFromAxisAngle(c.ax, rnd() * 6.283); c.live = true;
      }
    },
    update: function (dt, kill) {
      var any = false;
      for (i = 0; i < N; i++) {
        var c = cs[i]; if (!c.live) continue;
        var ox = c.p.x, oz = c.p.z, rc = c.sc * 0.4;
        c.v.y -= G * dt; c.p.x += c.v.x * dt; c.p.y += c.v.y * dt; c.p.z += c.v.z * dt;
        var oy = c.p.y - c.v.y * dt, h = pit.heightAt(c.p.x, c.p.z), fl = (h.inside ? h.y : 0) + rc, ground = false;
        if (c.p.y < fl) {
          // The surface here is higher than the chunk was a moment ago: it ran into a wall, so turn back.
          if (h.inside && fl > oy + s * 0.12) { c.p.x = ox; c.p.z = oz; c.v.x *= -0.4; c.v.z *= -0.4; h = pit.heightAt(ox, oz); fl = (h.inside ? h.y : 0) + rc; }
          if (c.p.y < fl) {
            c.p.y = fl; ground = true;
            if (c.v.y < 0) { c.v.y = -c.v.y * 0.32; if (c.v.y < s * 0.9) c.v.y = 0; }
          }
        }
        if (ground) {
          var dx = c.p.x - at.x, dz = c.p.z - at.z, dl = Math.sqrt(dx * dx + dz * dz) || 1, fr = Math.max(0, 1 - 2.4 * dt);
          c.v.x = c.v.x * fr - dx / dl * s * 2.6 * dt; c.v.z = c.v.z * fr - dz / dl * s * 2.6 * dt; c.w *= Math.max(0, 1 - 2 * dt);
        }
        var df = clamp01(-c.p.y / depthW);
        if (df > 0.97 || kill <= 0) { c.live = false; im.setMatrixAt(i, ZERO); continue; }
        any = true;
        var z = c.sc * (1 - 0.8 * smooth(0.05, 0.85, df)) * kill;
        c.q.premultiply(dq.setFromAxisAngle(c.ax, c.w * dt));
        m4.compose(c.p, c.q, sc.set(z, z * c.ky, z * c.kz)); im.setMatrixAt(i, m4);
      }
      im.instanceMatrix.needsUpdate = true; return any;
    } };
}

// A pool of points that bursts are fired into, so many small bursts cost one draw call. depthTest off is
// for things that must show over everything; order below the floor plane's (0) draws a particle inside
// the pit, where the floor's depth would otherwise hide it.
function abyssPool(st, n, o) {
  var T = st.T, pos = new Float32Array(n * 3), col = new Float32Array(n * 4), vel = new Float32Array(n * 3), age = new Float32Array(n), life = new Float32Array(n), ph = new Float32Array(n), cur = 0, rnd = Math.random;
  var cs = o.colors.map(function (h) { return new T.Color(h).multiplyScalar(o.boost || 1); }), rr = mulberry(o.seed || 5);
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('color', new T.BufferAttribute(col, 4));
  var pts = new T.Points(geo, new T.PointsMaterial({ size: st.ptSize(o.size), map: o.tex, transparent: true, depthWrite: false, depthTest: o.depthTest !== false, vertexColors: true, sizeAttenuation: true,
    blending: o.add ? T.AdditiveBlending : T.NormalBlending, toneMapped: !o.add }));
  pts.frustumCulled = false; pts.renderOrder = o.order; st.scene.add(pts);
  return { obj: pts,
    // Seeded, so the same throw always gives the same frames.
    emit: function (k, fn) {
      for (var j = 0; j < k; j++) {
        var i = cur, sp = fn(j, rr), c = cs[j % cs.length]; cur = (cur + 1) % n;
        pos[i * 3] = sp.p[0]; pos[i * 3 + 1] = sp.p[1]; pos[i * 3 + 2] = sp.p[2]; vel[i * 3] = sp.v[0]; vel[i * 3 + 1] = sp.v[1]; vel[i * 3 + 2] = sp.v[2];
        age[i] = -(sp.delay || 0); life[i] = sp.life; ph[i] = rr() * 6.283;
        col[i * 4] = c.r; col[i * 4 + 1] = c.g; col[i * 4 + 2] = c.b; col[i * 4 + 3] = 0;
      }
    },
    update: function (dt, fade) {
      var g = o.g || 0, dd = Math.max(0, 1 - (o.drag || 0) * dt), fl = o.floor;
      for (var i = 0; i < n; i++) {
        if (life[i] <= 0) continue;
        age[i] += dt;
        if (age[i] < 0) { col[i * 4 + 3] = 0; continue; }
        var k = age[i] / life[i], j = i * 3;
        if (k >= 1) { col[i * 4 + 3] = 0; life[i] = 0; continue; }
        vel[j + 1] -= g * dt; vel[j] *= dd; vel[j + 1] *= dd; vel[j + 2] *= dd;
        pos[j] += vel[j] * dt; pos[j + 1] += vel[j + 1] * dt; pos[j + 2] += vel[j + 2] * dt;
        if (fl != null && pos[j + 1] < fl) { pos[j + 1] = fl; vel[j + 1] *= -0.3; vel[j] *= 0.7; vel[j + 2] *= 0.7; }
        var a = o.alpha * fade * smooth(0, o.rise || 0.12, k) * (1 - smooth(o.fall || 0.55, 1, k));
        if (o.twinkle) a *= 0.2 + 0.8 * Math.pow(0.5 + 0.5 * Math.sin(age[i] * o.twinkle + ph[i]), 2);
        col[i * 4 + 3] = a;
      }
      geo.attributes.position.needsUpdate = true; geo.attributes.color.needsUpdate = true;
    } };
}

// The largest mouth radius at each angle: the pit never reaches a die that has to stay in view.
function abyssLimits(NA, at, R, minR, holes) {
  var lim = new Float32Array(NA), sm = new Float32Array(NA), j, pass;
  for (j = 0; j < NA; j++) {
    var an = j / NA * 6.2832, dx = Math.cos(an), dz = Math.sin(an), r = R;
    holes.forEach(function (h) {
      var cx = h.x - at.x, cz = h.z - at.z, proj = cx * dx + cz * dz, rr = h.r * 1.08, perp2 = cx * cx + cz * cz - proj * proj;
      if (proj > 0 && perp2 < rr * rr) r = Math.min(r, Math.max(minR, (proj - Math.sqrt(rr * rr - perp2)) / 1.1));
    });
    lim[j] = r;
  }
  // Round the notch off a little, never past the limit.
  for (pass = 0; pass < 2; pass++) { for (j = 0; j < NA; j++) sm[j] = Math.min(lim[j], (lim[(j + NA - 1) % NA] + lim[j] * 2 + lim[(j + 1) % NA]) / 4); lim.set(sm); }
  return lim;
}

function fxAbyss(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = st.q.detail === 'hd', t = 0, rnd = mulberry(7311), DUR = 4.6, i;
  var rad = abyssRadius(die), p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, at = new T.Vector3(p0.x, 0, p0.z);
  var holes = (o && o.clear) || [];
  // The beats: the crack gives way (BEAT), the die drops and is caught (SINK), hangs, then claws up in
  // three pulls (CLAW) and a heave (HEAVE) while the pit closes (CLOSE) and the die comes down (LAND).
  var T_BEAT = 0.95, T_SINK = 1.0, T_CLAW = 2.8, T_CLOSE = 3.3, T_HEAVE = 3.6, T_APEX = 3.8, T_LAND = 3.98;
  var R = rad * 2.95, NA = hd ? 112 : 72, U = { depth: ABYSS_DEPTH * s, depthWorld: ABYSS_DEPTH * s };
  var v3 = new T.Vector3(), qa = new T.Quaternion(), qy = new T.Quaternion(), AX = new T.Vector3(), Y = new T.Vector3(0, 1, 0);

  // The page layer: cracks painted as data over the first frames, then revealed by uniforms.
  var FW = rad * 13, C = hd ? 1024 : 512, kpx = C / FW, cv = canvas(C), cg = cv.getContext('2d');
  cg.fillStyle = '#000'; cg.fillRect(0, 0, C, C); cg.lineCap = 'round'; cg.lineJoin = 'round';
  var cr = abyssCrackSegs(C, rnd, hd, rad * 0.9 * kpx, rad * 5.2 * kpx), batches = abyssCrackBatches(cr), painted = 0, perFrame = Math.ceil(batches.length / 10);
  var tex = new T.CanvasTexture(cv); tex.anisotropy = 4;
  var layerMat = abyssLayerMat(st, tex, at, holes), lu = layerMat.uniforms;
  lu.uDimR.value = rad * 5.2; lu.uScarR.value = R * 0.62;
  var layer = new T.Mesh(new T.PlaneGeometry(FW, FW), layerMat);
  layer.rotation.x = -Math.PI / 2; layer.position.set(at.x, 0.008, at.z); layer.renderOrder = -10; layer.frustumCulled = false; st.scene.add(layer);

  // The pit and what falls into it.
  var pit = abyssPit(st, hd, at, s, abyssLimits(NA, at, R, rad * 1.3, holes), rnd, U), pu = pit.mat.userData.uni;
  st.scene.add(pit.mesh);
  var deb = abyssDebris(st, hd, s, at, pit, U); st.scene.add(deb.mesh);
  var dust = abyssPool(st, hd ? 120 : 60, { size: s * 1.5, tex: spriteTex(T), add: false, colors: ['#7a7686', '#5f5b6b', '#8f8b9a'], g: -s * 0.25, drag: 1.5, alpha: 0.26, order: 4, rise: 0.2, fall: 0.4, seed: 11 });
  var sparks = abyssPool(st, hd ? 90 : 45, { size: s * 0.26, tex: starTex(T), add: true, colors: [ABYSS_ICE, ABYSS_GLOW], boost: 1.3, drag: 0.9, g: s * 16, twinkle: 24, alpha: 1, order: 6, seed: 12, floor: 0.04 });
  // Cold motes drift up out of the deep; drawn before the floor plane so those below the page still show.
  var motes = abyssPool(st, hd ? 70 : 35, { size: s * 0.2, tex: spriteTex(T), add: true, colors: [ABYSS_GLOW, ABYSS_ICE], boost: 1.1, drag: 0.2, g: -s * 0.5, alpha: 0.9, order: -4, rise: 0.25, fall: 0.5, seed: 13 });
  var halo = glow(st, spriteTex(T), ABYSS_GLOW, s * 3), flare = glow(st, starTex(T), ABYSS_ICE, s * 5);
  halo.renderOrder = 5; halo.material.depthTest = false; flare.renderOrder = 7; flare.material.depthTest = false; st.scene.add(halo, flare);
  // One cold light deep in the pit lights the rock and the chunks from below; Basic has none.
  var lamp = hd ? new T.PointLight(ABYSS_GLOW, 0, rad * 14, 1.4) : null;
  if (lamp) { lamp.position.set(at.x, -U.depthWorld * 0.9, at.z); st.scene.add(lamp); }

  // The page is a clipping plane for the die: what has sunk below it is not drawn. A material that other
  // dice use too (a shell shared across a set) is clipped only while this die's own meshes draw with it,
  // so the plane never reaches the other dice.
  var plane = new T.Plane(new T.Vector3(0, 1, 0), 0.02), planes = [plane], clipped = [], hooks = [], others = [];
  st.dice.forEach(function (d) {
    if (d !== die) d.group.traverse(function (ob) { (ob.material ? (Array.isArray(ob.material) ? ob.material : [ob.material]) : []).forEach(function (m) { others.push(m); }); });
  });
  var nm = abyssNumeral(die, fi, th), pale = nm.pale, coldCol = pale ? '#1c4fd6' : ABYSS_GLOW, finalCol = pale ? '#1c4fd6' : mix(ABYSS_GLOW, ABYSS_ICE, 0.25);
  g.traverse(function (ob) {
    var shared = [];
    (ob.material ? (Array.isArray(ob.material) ? ob.material : [ob.material]) : []).forEach(function (m) {
      if (clipped.indexOf(m) >= 0 || shared.indexOf(m) >= 0 || (m.isShaderMaterial && !m.clipping)) return;
      if (others.indexOf(m) >= 0) shared.push(m); else { m.clippingPlanes = planes; clipped.push(m); }
    });
    if (!shared.length) return;
    var h = { ob: ob, bOwn: Object.prototype.hasOwnProperty.call(ob, 'onBeforeRender'), aOwn: Object.prototype.hasOwnProperty.call(ob, 'onAfterRender'), b: ob.onBeforeRender, a: ob.onAfterRender };
    ob.onBeforeRender = function (r, sc, cam, geo, mat) { if (shared.indexOf(mat) >= 0) mat.clippingPlanes = planes; h.b.apply(this, arguments); };
    ob.onAfterRender = function (r, sc, cam, geo, mat) { h.a.apply(this, arguments); if (shared.indexOf(mat) >= 0) mat.clippingPlanes = null; };
    hooks.push(h);
  });
  var fade0 = die.fade;

  // The die's motion. tilt about a horizontal axis, which turns a little with each pull.
  var phi0 = rnd() * 6.283, TILT = 0.36, pulses = [[2.8, 0.34, 0.14], [3.1, 0.40, 0.14], [3.38, 0.42, 0.10]], fired = [false, false, false], heaved = false, landed = false, beat = false, drip = 0, dustT = 0;
  function pullSum(tt) { var r = 0; pulses.forEach(function (p) { r += p[1] * out3((tt - p[0]) / 0.14) - p[2] * smooth(p[0] + 0.14, p[0] + 0.3, tt); }); return r; }
  var r0 = pullSum(T_HEAVE);
  // How far back up the die is: 0 at the bottom, 1 at the page, past 1 on the heave's hop.
  function riseAt(tt) {
    if (tt < T_CLAW) return 0;
    if (tt < T_HEAVE) return pullSum(tt);
    if (tt < T_APEX) return r0 + (1.28 - r0) * out3((tt - T_HEAVE) / 0.2);
    if (tt < T_LAND) { var x = (tt - T_APEX) / (T_LAND - T_APEX); return 1.28 - 0.28 * x * x; }
    return 1;
  }
  function pullPhase(tt) { var a = 0; pulses.forEach(function (p) { a += smooth(p[0], p[0] + 0.2, tt); }); return a; }
  function pullKick(tt) { var a = 0; pulses.forEach(function (p) { var x = (tt - p[0]) / 0.3; if (x > 0 && x < 1) a += Math.sin(x * Math.PI); }); return a; }
  function okAt(x, z) { for (var q = 0; q < holes.length; q++) if (Math.hypot(x - holes[q].x, z - holes[q].z) < holes[q].r * 1.2) return false; return true; }

  // Rock shed from the lip, round angle a0 within spread.
  function shed(n, a0, spread, big) {
    deb.emit(n, function (j, r) {
      var an = a0 + (r() - 0.5) * spread, rr = pit.rimAt(an) * (0.86 + 0.12 * r()), sp = s * (0.3 + r() * 0.9), ca = Math.cos(an), sa = Math.sin(an);
      return { x: at.x + ca * rr, y: 0.06 + r() * 0.12, z: at.z + sa * rr, vx: -ca * sp * 0.7 + (r() - 0.5) * s * 0.7, vy: s * (0.3 + r() * 1.4), vz: -sa * sp * 0.7 + (r() - 0.5) * s * 0.7,
        sc: s * (big ? 0.2 + r() * 0.2 : 0.07 + r() * 0.15), w: 3 + r() * 10 };
    });
  }
  function puff(n, a0, spread, r0p, rr, up) {
    dust.emit(n, function (j, r) {
      var an = a0 + (r() - 0.5) * spread, d = r0p + r() * rr, ca = Math.cos(an), sa = Math.sin(an), sp = s * (0.4 + r() * 1.4);
      return { p: [at.x + ca * d, 0.12 + r() * 0.2, at.z + sa * d], v: [ca * sp, s * (0.2 + r() * up), sa * sp], life: 1.1 + r() * 0.9, delay: r() * 0.08 };
    });
  }
  function spark(n, a0, spread, r0p, up) {
    sparks.emit(n, function (j, r) {
      var an = a0 + (r() - 0.5) * spread, ca = Math.cos(an), sa = Math.sin(an), sp = s * (1.6 + r() * 4);
      return { p: [at.x + ca * r0p, 0.1, at.z + sa * r0p], v: [ca * sp, s * (2 + r() * up), sa * sp], life: 0.45 + r() * 0.5, delay: r() * 0.05 };
    });
  }

  return { update: function (dt) {
    t += dt;
    var closeK = clamp01((t - T_CLOSE) / 0.75), open = t < T_CLOSE ? out3((t - T_BEAT) / 0.55) : 1 - closeK * closeK * (3 - 2 * closeK);
    var deep = out3((t - T_BEAT - 0.05) / 0.95) * (1 - out3(clamp01(closeK * 1.25)));

    // The die: tremble, drop and get caught, hang, claw up, heave, settle.
    var x = (t - T_SINK) / 0.95, kd = x <= 0 ? 0 : 1 - Math.exp(-3.4 * x) * (Math.cos(6.4 * x) + 0.531 * Math.sin(6.4 * x));
    var sunk = kd * (1 - riseAt(t)), sk = clamp01(sunk), hang = smooth(T_SINK + 1.0, T_SINK + 1.2, t) * (1 - smooth(T_CLAW - 0.1, T_CLAW, t));
    if (t >= T_LAND) { g.position.copy(p0); g.quaternion.copy(q0); }
    else {
      var tre = s * 0.03 * smooth(0.15, 0.9, t) * (t < T_BEAT + 0.05 ? 1 : 0.3), th1 = TILT * sk + 0.045 * Math.sin(t * 3.3) * sk * hang + 0.1 * pullKick(t) * sk;
      g.position.set(p0.x + (rnd() - 0.5) * tre, y0 - y0 * sunk + 0.035 * s * Math.sin(t * 4.2) * sk * hang + (rnd() - 0.5) * tre * 0.4, p0.z + (rnd() - 0.5) * tre);
      if (th1 === 0 && tre === 0) g.quaternion.copy(q0);
      else {
        var phi = phi0 + 0.5 * pullPhase(t) + 0.22 * Math.sin(t * 2.1) * sk;
        AX.set(Math.cos(phi), 0, Math.sin(phi));
        g.quaternion.copy(q0).premultiply(qy.setFromAxisAngle(Y, 0.16 * Math.sin(t * 2.6 + 1) * sk * hang + 0.1 * pullPhase(t) * sk)).premultiply(qa.setFromAxisAngle(AX, th1));
      }
    }
    die.fade = 1 - 0.75 * sk;

    // The 1: it chills and gutters, flashes as the crack gives way, burns cold while the die hangs.
    var nc, nk;
    if (t < T_BEAT) { nc = mix(th.burn, coldCol, smooth(0, 0.6, t)); nk = (1.6 + 0.5 * smooth(0, 0.6, t)) * (0.78 + 0.22 * Math.sin(t * 23) * Math.sin(t * 7.1 + 1)); }
    else if (t < T_LAND) { nc = coldCol; nk = 2 + 1.8 * Math.exp(-(t - T_BEAT) * 4) + 0.45 * Math.sin(t * 5) * hang; }
    else { nc = finalCol; nk = 1.7 + 1.6 * Math.exp(-(t - T_LAND) * 3); }
    nm.set(nc, nk);
    if (pale) shadeDie(die, 1 - 0.14 * smooth(0, 0.5, t) - 0.1 * sk);
    tintDie(die, ABYSS_DEEP, (0.08 * smooth(0.3, 1, t) + 0.12 * sk) * (1 - smooth(T_LAND, DUR - 0.2, t)));

    // The page: cracks reach out, the pit opens over them, then they fade with the scar.
    if (painted < batches.length) {
      for (var b = 0; b < perFrame && painted < batches.length; b++) abyssPaintBatch(cg, batches[painted++]);
      if (painted >= batches.length) tex.needsUpdate = true;
    }
    lu.uTime.value = t;
    lu.uGrow.value = painted >= batches.length ? 1 - Math.pow(1 - clamp01((t - 0.12) / 0.85), 1.8) : 0;
    lu.uCrackK.value = (1 - 0.5 * smooth(T_CLOSE, T_LAND, t)) * (1 - smooth(T_LAND, DUR - 0.1, t));
    lu.uGlow.value = smooth(0.1, 0.9, t) * (1 - smooth(T_CLOSE, T_LAND - 0.1, t)) * (0.85 + 0.15 * Math.sin(t * 6));
    lu.uDim.value = 0.5 * smooth(0, 0.9, t) * (1 - 0.75 * smooth(T_CLOSE, T_LAND, t)) * (1 - smooth(T_LAND + 0.1, DUR, t));
    lu.uScar.value = smooth(T_CLOSE + 0.1, T_CLOSE + 0.6, t) * (1 - smooth(T_LAND - 0.1, DUR, t));
    var ka = clamp01((t - T_BEAT) / 1.0), kb = clamp01((t - T_LAND) / (DUR - T_LAND));
    lu.uRingA.value.set(rad * (1 + out3(ka) * 3.6), rad * 0.7, t < T_BEAT ? 0 : 0.9 * Math.pow(1 - ka, 1.3));
    lu.uRingB.value.set(rad * (0.9 + out3(kb) * 3.0), rad * 0.7, t < T_LAND ? 0 : 0.5 * Math.pow(1 - kb, 1.5));
    pit.mesh.visible = open > 0.002;
    if (pit.mesh.visible) pit.update(open, deep);
    pu.uGlow.value = smooth(T_BEAT, T_BEAT + 1, t) * (0.8 + 0.2 * Math.sin(t * 5)) * (1 + 0.6 * hang) * (1 - smooth(T_CLOSE, T_LAND, t));
    if (lamp) { lamp.intensity = 3 * pu.uGlow.value * smooth(T_BEAT, T_BEAT + 0.5, t) * (1 - smooth(T_CLOSE, T_LAND, t)); lamp.position.y = -U.depthWorld * 0.9 * deep; }

    // Rock and dust.
    if (t >= T_BEAT - 0.05 && !beat) {
      beat = true; shed(hd ? 16 : 9, 0, 6.28, true); puff(hd ? 26 : 13, 0, 6.28, R * 0.96, rad * 0.8, 1.0); spark(hd ? 46 : 22, 0, 6.28, rad * 1.1, 5);
    }
    if (t > T_BEAT && t < T_BEAT + 0.8) { var n1 = Math.round(dt * 60 * (hd ? 1.6 : 0.9)); if (n1) shed(n1, rnd() * 6.28, 0.6, false); }
    // Dust blown up out of the cracks while they open.
    if (t > 0.2 && t < T_BEAT && (dustT -= dt) <= 0) {
      dustT = hd ? 0.045 : 0.09;
      var da = rnd() * 6.283, dd = rad * (1.4 + rnd() * 3.2);
      if (okAt(at.x + Math.cos(da) * dd, at.z + Math.sin(da) * dd)) dust.emit(1, function (j, r) { return { p: [at.x + Math.cos(da) * dd, 0.1, at.z + Math.sin(da) * dd], v: [(r() - 0.5) * s * 0.4, s * (0.4 + r() * 0.5), (r() - 0.5) * s * 0.4], life: 0.9 + r() * 0.6 }; });
    }
    if (t > T_SINK + 0.6 && t < T_CLAW && (drip -= dt) <= 0) { drip = 0.28; shed(1, rnd() * 6.28, 6.28, false); }
    if (open > 0.4 && t < T_CLOSE) {
      var nm1 = Math.round(dt * 60 * (hd ? 0.7 : 0.35) * (rnd() < 0.5 ? 1 : 0));
      if (nm1) motes.emit(nm1, function (j, r) { var a2 = r() * 6.283, rr = r() * R * 0.5 * open; return { p: [at.x + Math.cos(a2) * rr, -s * (1 + r() * 6), at.z + Math.sin(a2) * rr], v: [(r() - 0.5) * s * 0.3, s * (0.5 + r() * 0.8), (r() - 0.5) * s * 0.3], life: 2.2 + r() * 1.4 }; });
    }
    var lean = phi0 - Math.PI / 2 + 0.5 * pullPhase(t);
    for (i = 0; i < 3; i++) if (!fired[i] && t >= pulses[i][0]) { fired[i] = true; shed(hd ? 6 : 4, lean, 1.1, i > 0); puff(hd ? 8 : 4, lean, 1.2, rad * 1.2, rad * 0.6, 0.8); spark(hd ? 14 : 7, lean, 0.8, rad * 0.95, 4); }
    if (!heaved && t >= T_HEAVE) { heaved = true; shed(hd ? 9 : 5, lean, 2.2, true); puff(hd ? 14 : 7, lean, 2.2, rad * 1.2, rad * 0.8, 1.2); spark(hd ? 24 : 12, lean, 2.2, rad, 6); }
    if (!landed && t >= T_LAND) { landed = true; puff(hd ? 20 : 10, 0, 6.28, rad * 1.1, rad * 0.5, 0.5); spark(hd ? 16 : 8, 0, 6.28, rad * 1.05, 3); }
    var killK = 1 - smooth(T_CLOSE + 0.1, T_LAND + 0.2, t);
    deb.update(dt, killK);
    var pf = 1 - smooth(DUR - 0.3, DUR, t); dust.update(dt, pf); sparks.update(dt, pf); motes.update(dt, pf);

    // Light: a halo over the 1, and a flare as the crack gives way and as the die lands.
    halo.position.set(g.position.x, g.position.y + rad * 0.8, g.position.z);
    halo.material.opacity = (0.12 * smooth(0.2, 0.9, t) + 0.07 * sk) * (1 - smooth(T_LAND, DUR - 0.1, t));
    var fl = t >= T_BEAT ? 0.9 * Math.exp(-(t - T_BEAT) * 5) : 0;
    if (t >= T_LAND) fl += 0.3 * Math.exp(-(t - T_LAND) * 6);
    flare.position.set(g.position.x, g.position.y + rad * 0.9, g.position.z);
    flare.material.opacity = Math.min(0.7, fl * 0.7); flare.scale.setScalar(s * 5 * (0.8 + 0.4 * Math.min(1, fl)));

    if (t >= DUR) {
      clipped.forEach(function (m) { m.clippingPlanes = null; });
      hooks.forEach(function (h) {
        if (h.bOwn) h.ob.onBeforeRender = h.b; else delete h.ob.onBeforeRender;
        if (h.aOwn) h.ob.onAfterRender = h.a; else delete h.ob.onAfterRender;
      });
      g.position.copy(p0); g.quaternion.copy(q0);
      if (fade0 == null) delete die.fade; else die.fade = fade0;
      tintDie(die, th.burn, 0);
      nm.set(finalCol, pale ? 2 : 1.6);
      if (pale) shadeDie(die, 0.9);
      deb.mesh.dispose(); tex.dispose();
      drop(st, [layer, pit.mesh, deb.mesh, dust.obj, sparks.obj, motes.obj, halo, flare, lamp]);
      return false;
    }
    return true;
  } };
}

// ---------- The echo ----------
// Dark rings spread across the page under a die, as if the abyss stirs below it, and the die dips and
// rises. The same shader as the grand's rings; one plane per echo, drawn before the dice.
function abyssRippleMat(st, at, center) {
  var T = st.T, rg = [], i;
  for (i = 0; i < 4; i++) rg.push(new T.Vector3(0, 1, 0));
  var uni = { uCenter: { value: new T.Vector2(center.x, center.z) }, uRg: { value: rg }, uPool: { value: new T.Vector2(1, 0) }, uDir: { value: new T.Vector2(1, 0) }, uArc: { value: -2 }, uTime: { value: 0 },
    uLin: { value: st.cine ? 1 : 0 }, uCold: { value: new T.Color(ABYSS_GLOW) }, uInk: { value: new T.Color(ABYSS_INK) } };
  return new T.ShaderMaterial({ uniforms: uni, transparent: false, depthWrite: false, toneMapped: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor,
    vertexShader: 'varying vec3 vW; void main(){ vec4 w = modelMatrix * vec4(position, 1.0); vW = w.xyz; gl_Position = projectionMatrix * viewMatrix * w; }',
    fragmentShader: [
      'uniform vec2 uCenter, uDir, uPool; uniform vec3 uRg[4]; uniform vec3 uCold, uInk; uniform float uArc, uTime, uLin; varying vec3 vW;', ABYSS_GLSL, ABYSS_RING,
      'void main(){',
      ' vec2 q = vW.xz - uCenter; float r = length(q); vec2 au = q / max(r, 0.0001);',
      // The rings wobble, so they are never perfect circles.
      ' float rw = r / (1.0 + 0.16 * (abN(au * 2.0 + vec2(uTime * 0.7, uTime * 0.3)) - 0.5));',
      ' float mask = 1.0; if (uArc > -1.5) mask = smoothstep(uArc - 0.1, uArc + 0.15, dot(q / max(r, 0.0001), uDir));',
      ' vec2 a = vec2(0.0); for (int i = 0; i < 4; i++) a += abRing(rw, uRg[i]);',
      ' a *= mask;',
      ' float pool = uPool.y * smoothstep(uPool.x, uPool.x * 0.3, r);',
      ' float dk = clamp(a.x * 0.6 + pool * 0.5, 0.0, 0.9);',
      ' dk = 1.0 - pow(1.0 - dk, mix(1.0, 2.0, uLin));',
      ' gl_FragColor = vec4(uInk * dk + uCold * (a.y * 0.2 + pool * 0.06), dk);',
      ' #include <colorspace_fragment>',
      '}'].join('\n') });
}

function fxAbyssEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = st.q.detail === 'hd', t = 0, rad = abyssRadius(die), rnd = mulberry(5521);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, at = new T.Vector3(p0.x, 0, p0.z), own = [];
  // With a grand die: a dark ripple first runs across the page from its pit toward this die.
  var src = e && e.from ? e.from.group.position : null, gx = src ? src.x : 0, gz = src ? src.z : 0, dist = 0, ux = 1, uz = 0, travel = 0, rs = 0, tm = null, tu = null;
  if (src) {
    dist = Math.hypot(at.x - gx, at.z - gz); ux = (at.x - gx) / (dist || 1); uz = (at.z - gz) / (dist || 1);
    travel = clamp01((dist - 1) / (s * 16)) * 0.3 + 0.3; rs = (e.from.size || s) * 1.5;
    tm = abyssRippleMat(st, at, { x: gx, z: gz }); tu = tm.uniforms; tu.uDir.value.set(ux, uz); tu.uArc.value = 0.8;
    var tp = new T.Mesh(new T.PlaneGeometry((dist + rad * 4) * 2, (dist + rad * 4) * 2), tm);
    tp.rotation.x = -Math.PI / 2; tp.position.set(gx, 0.01, gz); tp.renderOrder = -9; tp.frustumCulled = false; st.scene.add(tp); own.push(tp);
  }
  var DUR = travel + 1.35, lm = abyssRippleMat(st, at, at), lu = lm.uniforms, lp = new T.Mesh(new T.PlaneGeometry(rad * 8, rad * 8), lm);
  lp.rotation.x = -Math.PI / 2; lp.position.set(at.x, 0.01, at.z); lp.renderOrder = -9; lp.frustumCulled = false; st.scene.add(lp); own.push(lp);
  var nm = abyssNumeral(die, fi, th), pale = nm.pale, coldCol = pale ? '#1c4fd6' : ABYSS_GLOW, finalCol = pale ? '#1c4fd6' : mix(ABYSS_GLOW, ABYSS_ICE, 0.25), phi = rnd() * 6.283;
  var AX = new T.Vector3(Math.cos(phi), 0, Math.sin(phi)), qa = new T.Quaternion();

  return { update: function (dt) {
    t += dt;
    if (tm) {
      var x = clamp01(t / travel), R = rs + (dist + rad - rs) * (1 - Math.pow(1 - x, 1.6));
      tu.uTime.value = t; tu.uRg.value[0].set(R, rad * 0.8, t > travel + 0.2 ? 0 : 0.9 * (1 - 0.5 * x) * (1 - smooth(travel, travel + 0.2, t)));
    }
    var lt = t - travel, k, ring = lu.uRg.value;
    lu.uTime.value = t;
    for (k = 0; k < 3; k++) {
      var age = lt - 0.22 * k;
      ring[k].set(rad * (0.9 + out3(age / 1.0) * 1.9), rad * (0.45 + 0.3 * Math.max(0, age)), age <= 0 ? 0 : 0.85 * Math.pow(1 - clamp01(age / 1.1), 1.4));
    }
    lu.uPool.value.set(rad * 1.9, 0.5 * env4(lt, 0, 0.25, 0.6, 1.3));
    // The die dips and rises as the rings leave it.
    var dip = lt > 0 && lt < 0.8 ? Math.pow(Math.sin(Math.PI * lt / 0.8), 1.2) : 0;
    if (t >= DUR) {
      g.position.copy(p0); g.quaternion.copy(q0); tintDie(die, th.burn, 0); nm.set(finalCol, pale ? 1.6 : 1.2);
      own.forEach(function (m) { m.material.dispose(); }); drop(st, own);
      return false;
    }
    g.position.set(p0.x, y0 - 0.16 * s * dip, p0.z);
    if (dip > 0) g.quaternion.copy(q0).premultiply(qa.setFromAxisAngle(AX, 0.04 * dip)); else g.quaternion.copy(q0);
    nm.set(coldCol, (0.6 + 1.2 * dip) * smooth(travel - 0.1, travel + 0.2, t));
    tintDie(die, ABYSS_DEEP, 0.2 * dip);
    return true;
  } };
}
registerCrit('abyss', { name: 'The abyss', kind: 'fail',
  what: 'The page cracks under the die and gives way to a deep dark pit. The die sinks halfway, hangs, then claws its way back up as the pit closes, leaving a faint scar and the 1 lit a cold deep blue.',
  fx: fxAbyss, beat: 0.95, peak: 2.6, echo: fxAbyssEcho, echoWhat: 'Dark ripples spread out under the die, as if the abyss stirs beneath it, and the die dips a little and rises.' });
