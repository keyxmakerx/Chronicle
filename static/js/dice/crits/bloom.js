// ---------- Crit success: Wildbloom ----------
// Vines burst up out of the page round the 20, spiral up over the die and put out leaves and buds that
// open into flowers. Pollen bursts, petals drift down, and the vines sink back into the page, leaving the
// die clean with the 20 lit spring gold.
//
// All the stems are one mesh: each vine is a tube rebuilt every frame along a path made once at the start,
// grown by moving a front along it, so reaching, retreating and swaying cost one draw call. Leaves are one
// instanced mesh, and every petal (on a flower or drifting free) is one instance of a petal strip that a
// vertex shader bends open, so the flowers cost one more. Everything is clipped at the page, which is how
// a vine rises out of it and sinks back into it.
import {
  clamp01, drawBurn, drop, env4, glow, glowFace, glowNumeral, inout, mulberry, out3, registerCrit,
  smooth, spray, spriteTex, starTex, tintDie, worldOf
} from '../engine.js';

var BLOOM = {
  gold: '#ffd24a', amber: '#ff8a00', warm: '#ffe6a0',
  tints: ['#ff5f9a', '#ff8a3c', '#a56bff', '#ffc21a', '#ff4d6a', '#ff8fc2'],
  nodes: 9
};

// A gold glow washes out on a pale stone, so a pale die keeps its own number-glow colour (which its set chose to read
// on that stone) and the 20 is only gold on the darker ones.
function bloomNumeral(th) {
  var g = th && th.base && th.base.grad, sum = 0, n = 0, i, m;
  if (g) for (i = 0; i < g.length; i++) {
    m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(g[i]);
    if (m) { sum += (0.3 * parseInt(m[1], 16) + 0.59 * parseInt(m[2], 16) + 0.11 * parseInt(m[3], 16)) / 255; n++; }
  }
  return n && sum / n > 0.6 ? (th.burn || BLOOM.amber) : BLOOM.gold;
}

var BLOOM_NOISE = [
  'float blh(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float bln(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(blh(i), blh(i + vec2(1.0, 0.0)), u.x), mix(blh(i + vec2(0.0, 1.0)), blh(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float blf(vec2 p){ float s = 0.0, a = 0.5; for (int i = 0; i < 4; i++) { s += a * bln(p); p = p * 2.03 + 1.7; a *= 0.5; } return s; }'
].join('\n');
// A surface bump from a height h, in world units: tilts the normal by the slope of h across the screen.
var BLOOM_BUMP = [
  'vec3 blBump(vec3 p, vec3 n, float h){',
  '  vec3 sx = dFdx(p), sy = dFdy(p), r1 = cross(sy, n), r2 = cross(n, sx);',
  '  float det = dot(sx, r1);',
  '  vec3 gr = sign(det) * (dFdx(h) * r1 + dFdy(h) * r2);',
  '  return normalize(abs(det) * n - gr); }'
].join('\n');

// ---------- Materials ----------
// The stem: woody and ridged at the root, fresh green at the tip, red-brown at the nodes where leaves
// join, waxy, with a green light under the skin at the edges and a gold pulse of sap that runs up it
// while the flowers open.
function bloomStemMat(st, hd, clip) {
  var T = st.T, uni = { uTime: { value: 0 }, uSap: { value: 0 } };
  var mat = new T.MeshPhysicalMaterial({ color: '#ffffff', roughness: 0.5, metalness: 0, clearcoat: hd ? 0.3 : 0, clearcoatRoughness: 0.4, envMapIntensity: 0.8, clippingPlanes: clip });
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nattribute float aU; attribute float aA; attribute float aSd; varying float vU; varying float vA; varying float vSd;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvU = aU; vA = aA; vSd = aSd;');
    sh.fragmentShader = sh.fragmentShader
      .replace('#include <common>', '#include <common>\n#define BL_NODES ' + BLOOM.nodes + '.0\nuniform float uTime, uSap; varying float vU; varying float vA; varying float vSd;\n' + BLOOM_NOISE + '\n' + BLOOM_BUMP)
      .replace('#include <color_fragment>', [
        '#include <color_fragment>',
        'float mott = blf(vec2(vU * 32.0, vA * 6.0 + vSd * 13.0));',
        'float root = 1.0 - smoothstep(0.0, 0.34, vU);',
        'vec3 sc = mix(vec3(0.10, 0.26, 0.05), vec3(0.30, 0.58, 0.09), smoothstep(0.4, 0.95, vU));',
        'sc = mix(sc, vec3(0.17, 0.115, 0.06), root);',
        'sc *= 0.7 + 0.6 * mott;',
        'float nodeW = pow(0.5 + 0.5 * cos((vU * BL_NODES - 0.55) * 6.2832), 6.0);',
        'sc = mix(sc, vec3(0.30, 0.12, 0.06), nodeW * 0.3);',
        'float stripe = smoothstep(0.55, 0.9, sin(vA * 6.2832 * 3.0 + vU * 30.0) * 0.5 + 0.5);',
        'sc *= 1.0 - 0.15 * stripe * (1.0 - root);',
        'diffuseColor.rgb *= sc;'].join('\n'))
      .replace('#include <normal_fragment_maps>', [
        '#include <normal_fragment_maps>',
        'float bh = 0.5 * (sin(vA * 6.2832 * 3.0 + vU * 30.0) * 0.5 + 0.5) + 0.8 * blf(vec2(vU * 120.0, vA * 10.0 + vSd * 7.0)) + root * 0.9 * blf(vec2(vU * 40.0, vA * 7.0));',
        'normal = blBump(-vViewPosition, normal, bh * 0.012);'].join('\n'))
      .replace('#include <emissivemap_fragment>', [
        '#include <emissivemap_fragment>',
        'float fr = pow(1.0 - abs(dot(normalize(normal), normalize(vViewPosition))), 3.0);',
        'totalEmissiveRadiance += vec3(0.22, 0.42, 0.06) * fr * 0.55;',
        'float sap = pow(0.5 + 0.5 * sin(vU * 26.0 - uTime * 7.0 + vSd * 6.0), 5.0);',
        'totalEmissiveRadiance += vec3(0.9, 0.75, 0.2) * sap * uSap * (0.25 + 0.75 * smoothstep(0.1, 0.9, vU));'].join('\n'));
  };
  mat.userData.uni = uni;
  return mat;
}

// A leaf: veined, two-sided, translucent-looking. The vertex shader furls it into a spear and opens it
// out as aDat.x runs 0 to 1, and droops its tip when open.
function bloomLeafMat(st, clip) {
  var T = st.T, uni = { uTime: { value: 0 } };
  var mat = new T.MeshStandardMaterial({ color: '#ffffff', roughness: 0.46, metalness: 0, side: T.DoubleSide, envMapIntensity: 0.9, clippingPlanes: clip });
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nuniform float uTime; attribute vec2 aLp; attribute vec4 aDat; varying vec2 vLp; varying vec4 vDat;')
      .replace('#include <beginnormal_vertex>', [
        'vec3 objectNormal = vec3(normal);',
        'float lx = position.x, uf = aDat.x, cl = 1.0 - uf;',
        'objectNormal = normalize(objectNormal + vec3(-2.0 * lx * (1.1 * cl - 0.32 * uf), 0.0, 0.0));'].join('\n'))
      .replace('#include <begin_vertex>', [
        'vec3 transformed = vec3(position);',
        'transformed.z *= mix(0.14, 1.0, smoothstep(0.0, 1.0, uf));',
        'transformed.y += lx * lx * (1.1 * cl - 0.32 * uf) + 0.45 * cl * abs(position.z) + 0.02 * sin(lx * 6.0 + uTime * 2.6 + aDat.y * 6.0) * uf * lx;',
        'vLp = aLp; vDat = aDat;'].join('\n'));
    sh.fragmentShader = sh.fragmentShader
      .replace('#include <common>', '#include <common>\nvarying vec2 vLp; varying vec4 vDat;\n' + BLOOM_NOISE)
      .replace('#include <color_fragment>', [
        '#include <color_fragment>',
        'float ax = abs(vLp.y);',
        'vec3 lc = mix(vec3(0.045, 0.19, 0.035), vec3(0.22, 0.48, 0.06), smoothstep(0.0, 1.0, ax * 0.8 + vLp.x * 0.35));',
        'lc = mix(lc, vec3(0.36, 0.52, 0.05), vDat.z * 0.5);',
        'float rib = smoothstep(0.09, 0.0, ax) * smoothstep(1.0, 0.3, vLp.x);',
        'float lat = smoothstep(0.07, 0.0, abs(fract(vLp.x * 5.5 - ax * 1.4) - 0.5)) * smoothstep(1.0, 0.5, ax);',
        'lc = mix(lc, vec3(0.45, 0.64, 0.14), clamp(rib * 0.75 + lat * 0.3, 0.0, 1.0));',
        'lc *= 0.88 + 0.24 * blf(vLp * vec2(6.0, 3.0) + vDat.y * 10.0);',
        'diffuseColor.rgb *= lc;'].join('\n'))
      .replace('#include <emissivemap_fragment>', '#include <emissivemap_fragment>\ntotalEmissiveRadiance += diffuseColor.rgb * 0.2;');
  };
  mat.userData.uni = uni;
  return mat;
}

// A petal strip: aP is (open 0..1, elevation when open, curl when open, width), aT the tint and, in w, how
// far inside the flower it grows. The shader bends the strip along a circular arc from a closed bud (upright
// and curled in) to its open pose, cups it, and shades it from a warm base to its tint.
var BLOOM_PETAL_FN = [
  'void blPetal(vec2 lp, vec4 P, out vec3 pos, out vec3 nrm){',
  '  float x = lp.x, zn = lp.y, op = P.x;',
  '  float th0 = mix(1.5, P.y, op), k = mix(1.4, P.z, op), th = th0 + k * x;',
  '  vec2 q = abs(k) < 0.002 ? vec2(cos(th0), sin(th0)) * x : vec2(sin(th) - sin(th0), cos(th0) - cos(th)) / k;',
  '  float w = 0.5 * pow(max(sin(3.14159 * pow(x, 1.6)), 0.0), 0.6) * P.w;',
  '  float cupK = mix(0.3, 1.0, op);',
  '  vec2 nn = vec2(-sin(th), cos(th));',
  '  pos = vec3(q.x + 0.1, q.y, zn * w) + vec3(nn, 0.0) * (0.5 * w * zn * zn * cupK);',
  '  nrm = normalize(vec3(nn, -0.9 * zn * cupK)); }'
].join('\n');
function bloomPetalMat(st, clip) {
  var T = st.T, uni = { uGlow: { value: 0 } };
  var mat = new T.MeshStandardMaterial({ color: '#ffffff', roughness: 0.55, metalness: 0, side: T.DoubleSide, envMapIntensity: 0.9, clippingPlanes: clip });
  mat.onBeforeCompile = function (sh) {
    Object.assign(sh.uniforms, uni);
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nattribute vec4 aP; attribute vec4 aT; varying vec4 vTint; varying vec2 vLp;\n' + BLOOM_PETAL_FN)
      .replace('#include <beginnormal_vertex>', 'vec3 bPos; vec3 objectNormal; blPetal(position.xz, aP, bPos, objectNormal);\nvTint = aT; vLp = position.xz;')
      .replace('#include <begin_vertex>', 'vec3 transformed = bPos;');
    sh.fragmentShader = sh.fragmentShader
      .replace('#include <common>', '#include <common>\nuniform float uGlow; varying vec4 vTint; varying vec2 vLp;')
      .replace('#include <color_fragment>', [
        '#include <color_fragment>',
        'float px = vLp.x, pa = abs(vLp.y);',
        'vec3 pc = mix(vec3(1.0, 0.86, 0.42), vTint.rgb, smoothstep(0.02, 0.5, px));',
        'pc = mix(pc, vTint.rgb * vec3(1.2, 1.08, 1.1), smoothstep(0.55, 1.0, pa) * 0.45);',
        'pc *= 1.0 - 0.1 * (0.5 + 0.5 * cos(vLp.y * 15.0 + px * 2.0)) * smoothstep(0.1, 0.5, px);',
        'pc = mix(pc, vec3(0.2, 0.4, 0.07), smoothstep(0.16, 0.0, px) * (1.0 - vTint.a));',
        'pc *= 1.0 - 0.3 * vTint.a * (1.0 - px);',
        'diffuseColor.rgb *= pc;'].join('\n'))
      .replace('#include <emissivemap_fragment>', '#include <emissivemap_fragment>\ntotalEmissiveRadiance += diffuseColor.rgb * (0.1 + 0.45 * uGlow);');
  };
  mat.userData.uni = uni;
  return mat;
}

// ---------- Geometry ----------
// Leaf: x along 0..1 (a short stalk, then an ovate blade), z across; aLp is (x, -1..1 across).
function bloomLeafGeo(T, hd) {
  var SX = hd ? 9 : 6, SZ = hd ? 6 : 4, pos = [], nrm = [], lp = [], idx = [], i, j;
  for (i = 0; i <= SX; i++) for (j = 0; j <= SZ; j++) {
    var x = i / SX, zn = j / SZ * 2 - 1, tt = Math.max(0, (x - 0.1) / 0.9);
    var w = (x < 0.1 ? 0 : 0.27 * Math.pow(Math.max(0, Math.sin(Math.PI * Math.pow(tt, 0.75))), 0.8)) + 0.012, z = zn * w;
    pos.push(x, 0.34 * Math.abs(z), z); lp.push(x, zn);
    var nz = zn > 0 ? -0.34 : zn < 0 ? 0.34 : 0, nl = Math.hypot(1, nz); nrm.push(0, 1 / nl, nz / nl);
  }
  for (i = 0; i < SX; i++) for (j = 0; j < SZ; j++) { var a = i * (SZ + 1) + j, b = a + 1, c = a + SZ + 1, d = c + 1; idx.push(a, b, c, b, d, c); }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.Float32BufferAttribute(nrm, 3));
  geo.setAttribute('aLp', new T.Float32BufferAttribute(lp, 2)); geo.setIndex(idx);
  return geo;
}
// Petal: position is (x along 0..1, 0, -1..1 across); the shader shapes it.
function bloomPetalGeo(T, hd) {
  var SX = hd ? 9 : 6, SZ = hd ? 6 : 4, pos = [], idx = [], i, j;
  for (i = 0; i <= SX; i++) for (j = 0; j <= SZ; j++) pos.push(i / SX, 0, j / SZ * 2 - 1);
  for (i = 0; i < SX; i++) for (j = 0; j < SZ; j++) { var a = i * (SZ + 1) + j, b = a + 1, c = a + SZ + 1, d = c + 1; idx.push(a, b, c, b, d, c); }
  var geo = new T.BufferGeometry();
  geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setIndex(idx);
  return geo;
}

// ---------- Paths ----------
// A Catmull-Rom curve through waypoints (flat xyz list), resampled to SEG+1 evenly spaced points in out.
// Returns its length.
function bloomPath(pts, SEG, out) {
  var n = pts.length / 3, K = 40 * (n - 1), dense = new Float32Array((K + 1) * 3), cum = new Float32Array(K + 1), i, c, k;
  for (i = 0; i <= K; i++) {
    var f = i / K * (n - 1), sg = Math.min(n - 2, Math.floor(f)), t = f - sg, t2 = t * t, t3 = t2 * t;
    var i0 = Math.max(0, sg - 1), i1 = sg, i2 = sg + 1, i3 = Math.min(n - 1, sg + 2);
    for (c = 0; c < 3; c++) {
      var p0 = pts[i0 * 3 + c], p1 = pts[i1 * 3 + c], p2 = pts[i2 * 3 + c], p3 = pts[i3 * 3 + c];
      dense[i * 3 + c] = 0.5 * (2 * p1 + (-p0 + p2) * t + (2 * p0 - 5 * p1 + 4 * p2 - p3) * t2 + (-p0 + 3 * p1 - 3 * p2 + p3) * t3);
    }
    if (i) cum[i] = cum[i - 1] + Math.hypot(dense[i * 3] - dense[i * 3 - 3], dense[i * 3 + 1] - dense[i * 3 - 2], dense[i * 3 + 2] - dense[i * 3 - 1]);
  }
  var total = cum[K], j = 0;
  for (k = 0; k <= SEG; k++) {
    var want = total * k / SEG; while (j < K - 1 && cum[j + 1] < want) j++;
    var w = (want - cum[j]) / (cum[j + 1] - cum[j] || 1);
    for (c = 0; c < 3; c++) out[k * 3 + c] = dense[j * 3 + c] + (dense[(j + 1) * 3 + c] - dense[j * 3 + c]) * w;
  }
  return total;
}
// How far a ray from (cx, cz) along angle ps may run before it reaches a place to keep clear.
function bloomReach(cx, cz, ps, L, minL, clear) {
  var dx = Math.cos(ps), dz = Math.sin(ps);
  clear.forEach(function (c) {
    var rx = c.x - cx, rz = c.z - cz, along = rx * dx + rz * dz, off = Math.abs(rx * dz - rz * dx);
    if (along > 0 && off < c.r + 0.5) L = Math.min(L, Math.max(minL, along - Math.sqrt(Math.max(0, c.r * c.r - off * off)) - 0.4));
  });
  return L;
}
// The vine that coils up the die: bursts straight up out of the page beside it, then winds up over its
// shoulder on a sphere just outside the corners.
function bloomCoilPts(cx, yc, cz, s, phi0, sweep, rootR, rsph) {
  var pts = [], c = Math.cos, sn = Math.sin, k;
  pts.push(cx + s * rootR * c(phi0), -0.7 * s, cz + s * rootR * sn(phi0));
  pts.push(cx + s * rootR * 0.96 * c(phi0 + 0.04), 0.45 * s, cz + s * rootR * 0.96 * sn(phi0 + 0.04));
  var es = [-0.30, -0.08, 0.10, 0.26, 0.40], ps = [0.16, 0.40, 0.62, 0.82, 1.0];
  for (k = 0; k < 5; k++) { var e = es[k], ph = phi0 + sweep * ps[k], R = s * rsph; pts.push(cx + R * c(e) * c(ph), yc + R * sn(e), cz + R * c(e) * sn(ph)); }
  return pts;
}
// A vine that arches up and out over the page and droops toward it; reach 0..1 scales how far out it goes.
function bloomFountPts(cx, cz, s, phi, turn, reach) {
  var R = [2.1, 2.15, 2.45, 2.95, 3.35, 3.6], Y = [-0.6, 0.5, 1.7, 2.5, 2.1, 1.25], D = [0, 0.02, 0.15, 0.32, 0.5, 0.62], pts = [], i;
  for (i = 0; i < 6; i++) { var rr = s * (2.1 + (R[i] - 2.1) * reach), a = phi + D[i] * turn; pts.push(cx + rr * Math.cos(a), s * Y[i], cz + rr * Math.sin(a)); }
  return pts;
}
// A vine that creeps over the page, weaving, and lifts at its end to carry a flower.
function bloomRunPts(cx, cz, s, ps, L, side) {
  var dx = Math.cos(ps), dz = Math.sin(ps), sx = -dz, sz = dx, r0 = 1.4 * s, pts = [], i, n = 5;
  pts.push(cx + dx * r0, -0.45 * s, cz + dz * r0);
  pts.push(cx + dx * (r0 + 0.1 * s), 0.05 * s, cz + dz * (r0 + 0.1 * s));
  for (i = 1; i <= n; i++) {
    var f = i / n, d = r0 + 0.1 * s + (L - r0 - 0.1 * s) * f, m = side * 0.42 * s * Math.sin(f * 4.2 + ps) * f;
    var y = (0.07 + 0.04 * Math.sin(f * 9)) * s + (i === n ? 0.5 * s : i === n - 1 ? 0.2 * s : 0);
    pts.push(cx + dx * d + sx * m, y, cz + dz * d + sz * m);
  }
  return pts;
}

// ---------- The stems ----------
// n vines of SEG+1 rings of SID+1 vertices in one mesh. S[v] holds vine v's path; put() writes the tube for
// a front (0..1) that has grown along it, swayed, sunk and pushed out of the places to keep clear.
// P[v] and Tg[v] keep the positions and headings it wrote, for the leaves and flowers that sit on it.
function bloomVines(st, n, SEG, SID, clip, hd, order) {
  var T = st.T, V1 = SEG + 1, S1 = SID + 1, NV = n * V1 * S1, v, k, j;
  var pos = new Float32Array(NV * 3), nrm = new Float32Array(NV * 3), au = new Float32Array(NV), aa = new Float32Array(NV), asd = new Float32Array(NV), idx = [];
  var CS = new Float32Array(S1), SN = new Float32Array(S1), S = [], P = [], Tg = [], seed = [], hidden = [];
  for (j = 0; j < S1; j++) { CS[j] = Math.cos(j / SID * 6.2832); SN[j] = Math.sin(j / SID * 6.2832); }
  for (v = 0; v < n; v++) {
    S.push(new Float32Array(V1 * 3)); P.push(new Float32Array(V1 * 3)); Tg.push(new Float32Array(V1 * 3)); seed.push(v * 0.37 + 0.11); hidden.push(false);
    for (k = 0; k < V1; k++) for (j = 0; j < S1; j++) {
      var o = (v * V1 + k) * S1 + j; au[o] = k / SEG; aa[o] = j / SID; asd[o] = seed[v];
      if (k < SEG && j < SID) { var a = o, b = o + 1, c = o + S1, d = c + 1; idx.push(a, b, c, b, d, c); }
    }
  }
  var geo = new T.BufferGeometry(), pa = new T.BufferAttribute(pos, 3), na = new T.BufferAttribute(nrm, 3);
  pa.setUsage(T.DynamicDrawUsage); na.setUsage(T.DynamicDrawUsage);
  geo.setAttribute('position', pa); geo.setAttribute('normal', na); geo.setAttribute('aU', new T.BufferAttribute(au, 1));
  geo.setAttribute('aA', new T.BufferAttribute(aa, 1)); geo.setAttribute('aSd', new T.BufferAttribute(asd, 1)); geo.setIndex(idx);
  var mat = bloomStemMat(st, hd, clip), mesh = new T.Mesh(geo, mat);
  mesh.frustumCulled = false; mesh.castShadow = true; mesh.receiveShadow = true; mesh.renderOrder = order;
  function put(v, front, thick, time, sway, sinkY, clear, rb) {
    if (front <= 0.0005) { if (hidden[v]) return; hidden[v] = true; } else hidden[v] = false;
    var Sv = S[v], Pv = P[v], Tv = Tg[v], ph = seed[v] * 17, base = v * V1 * S1, k, j, i;
    for (k = 0; k < V1; k++) {
      var u = k / SEG, sw = sway * u * u, x = Sv[k * 3], y = Sv[k * 3 + 1], z = Sv[k * 3 + 2];
      x += sw * Math.sin(time * 1.9 + u * 5.2 + ph); z += sw * Math.cos(time * 1.55 + u * 4.1 + ph * 1.7);
      y += sw * 0.35 * Math.sin(time * 2.4 + u * 6 + ph * 0.6) - sinkY;
      for (i = 0; i < clear.length; i++) {
        var c = clear[i], dx = x - c.x, dz = z - c.z, d2 = dx * dx + dz * dz;
        if (d2 < c.r * c.r) { var dd = Math.sqrt(d2) || 1e-4; x = c.x + dx / dd * c.r; z = c.z + dz / dd * c.r; }
      }
      Pv[k * 3] = x; Pv[k * 3 + 1] = y; Pv[k * 3 + 2] = z;
    }
    // A frame carried along the path by projection, so the rings never twist.
    var nx = 0, ny = 1, nz = 0;
    for (k = 0; k < V1; k++) {
      var a0 = k ? k - 1 : 0, b0 = k < SEG ? k + 1 : SEG, tx = Pv[b0 * 3] - Pv[a0 * 3], ty = Pv[b0 * 3 + 1] - Pv[a0 * 3 + 1], tz = Pv[b0 * 3 + 2] - Pv[a0 * 3 + 2], tl = Math.sqrt(tx * tx + ty * ty + tz * tz);
      if (tl < 1e-6) { tx = 0; ty = 1; tz = 0; } else { tx /= tl; ty /= tl; tz /= tl; }
      Tv[k * 3] = tx; Tv[k * 3 + 1] = ty; Tv[k * 3 + 2] = tz;
      if (k === 0) { if (Math.abs(ty) > 0.9) { nx = 1; ny = 0; } else { nx = 0; ny = 1; } nz = 0; }
      var dn = nx * tx + ny * ty + nz * tz; nx -= tx * dn; ny -= ty * dn; nz -= tz * dn;
      var nl = Math.sqrt(nx * nx + ny * ny + nz * nz);
      if (nl < 1e-5) { nx = ty; ny = -tx; nz = 0; nl = Math.sqrt(nx * nx + ny * ny) || 1; }
      nx /= nl; ny /= nl; nz /= nl;
      var bx = ty * nz - tz * ny, by = tz * nx - tx * nz, bz = tx * ny - ty * nx;
      var u2 = k / SEG, kc = clamp01((front - u2) / 0.045), r = 0;
      if (kc > 0) {
        var nodeW = Math.pow(0.5 + 0.5 * Math.cos((u2 * BLOOM.nodes - 0.55) * 6.2832), 6);
        r = rb * thick * (0.17 + 0.83 * Math.pow(1 - u2, 0.8)) * (1 + 0.55 * Math.exp(-u2 * 28)) * (1 + 0.1 * nodeW) * (0.62 + 0.38 * clamp01((front - u2) / 0.35)) * Math.pow(Math.sin(kc * 1.5708), 0.8);
      }
      var o3 = (base + k * S1) * 3;
      for (j = 0; j < S1; j++, o3 += 3) {
        var ex = nx * CS[j] + bx * SN[j], ey = ny * CS[j] + by * SN[j], ez = nz * CS[j] + bz * SN[j];
        pos[o3] = Pv[k * 3] + ex * r; pos[o3 + 1] = Pv[k * 3 + 1] + ey * r; pos[o3 + 2] = Pv[k * 3 + 2] + ez * r;
        nrm[o3] = ex; nrm[o3 + 1] = ey; nrm[o3 + 2] = ez;
      }
    }
  }
  return { mesh: mesh, mat: mat, S: S, P: P, Tg: Tg, SEG: SEG, put: put, count: n,
    commit: function (time, sap) { pa.needsUpdate = true; na.needsUpdate = true; mat.userData.uni.uTime.value = time; mat.userData.uni.uSap.value = sap; } };
}

// ---------- The leaves ----------
// set() lays leaf i from its stalk at p, blade along d (unit), lying as flat to the camera as d allows,
// rolled by roll about d. len scales it, wk its width, unf is how far it has unfurled.
function bloomLeaves(st, n, hd, clip, order) {
  var T = st.T, geo = bloomLeafGeo(T, hd), aDat = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4);
  aDat.setUsage(T.DynamicDrawUsage); geo.setAttribute('aDat', aDat);
  var mat = bloomLeafMat(st, clip), mesh = new T.InstancedMesh(geo, mat, n);
  mesh.instanceMatrix.setUsage(T.DynamicDrawUsage); mesh.frustumCulled = false; mesh.castShadow = true; mesh.receiveShadow = true; mesh.renderOrder = order;
  var M = mesh.instanceMatrix.array, D = aDat.array; M.fill(0);
  return { mesh: mesh, mat: mat,
    hide: function (i) { for (var k = i * 16; k < i * 16 + 16; k++) M[k] = 0; },
    set: function (i, px, py, pz, dx, dy, dz, len, wk, roll, unf, seed, tint) {
      var yx = -dx * dy, yy = 1 - dy * dy, yz = -dz * dy, yl = Math.sqrt(yx * yx + yy * yy + yz * yz) || 1;
      yx /= yl; yy /= yl; yz /= yl;
      var cr = Math.cos(roll), sr = Math.sin(roll), wx = dy * yz - dz * yy, wy = dz * yx - dx * yz, wz = dx * yy - dy * yx;
      var ax = yx * cr + wx * sr, ay = yy * cr + wy * sr, az = yz * cr + wz * sr;
      var zx = dy * az - dz * ay, zy = dz * ax - dx * az, zz = dx * ay - dy * ax, m = i * 16, zl = len * wk;
      M[m] = dx * len; M[m + 1] = dy * len; M[m + 2] = dz * len; M[m + 3] = 0;
      M[m + 4] = ax * len; M[m + 5] = ay * len; M[m + 6] = az * len; M[m + 7] = 0;
      M[m + 8] = zx * zl; M[m + 9] = zy * zl; M[m + 10] = zz * zl; M[m + 11] = 0;
      M[m + 12] = px; M[m + 13] = py; M[m + 14] = pz; M[m + 15] = 1;
      var q = i * 4; D[q] = unf; D[q + 1] = seed; D[q + 2] = tint; D[q + 3] = 1;
    },
    commit: function (time) { mesh.instanceMatrix.needsUpdate = true; aDat.needsUpdate = true; mat.userData.uni.uTime.value = time; } };
}

// ---------- The flowers ----------
// Each flower is rings of petals round a centre. flowers[f] carries what the caller sets every frame:
// x, y, z (its base), size, vis (0 hidden .. 1 grown), and fixed things: q (its facing), tint, tO (when it
// opens), relT (when its petals let go in the gust), peelT (when a few let go early). The module opens the
// petals, lets them go, and flies each free petal until it fades. wind is { x, z, k }, set every frame.
var BLOOM_RINGS = [
  { n: 6, len: 1.00, w: 0.80, el: 0.10, k: -0.55, off: 0.00, dly: 0.00, inner: 0.0 },
  { n: 6, len: 0.80, w: 0.76, el: 0.55, k: -0.15, off: 0.50, dly: 0.14, inner: 0.4 },
  { n: 5, len: 0.58, w: 0.70, el: 1.05, k: 0.60, off: 0.25, dly: 0.28, inner: 0.85 }
];
function bloomFlowers(st, flowers, hd, clip, order, clear, rnd) {
  var T = st.T, rings = hd ? BLOOM_RINGS : [BLOOM_RINGS[0], { n: 5, len: 0.72, w: 0.76, el: 0.6, k: -0.1, off: 0.5, dly: 0.16, inner: 0.5 }];
  var slots = [], f, r, j;
  flowers.forEach(function (fl, fi) {
    rings.forEach(function (rg, ri) {
      for (j = 0; j < rg.n; j++) {
        var az = (j + rg.off) / rg.n * 6.2832 + (rnd() - 0.5) * 0.25, peel = ri === 0 && rnd() < 0.3;
        slots.push({ f: fi, rg: rg, ri: ri, az: az, open: 0, len: rg.len * (0.92 + rnd() * 0.16), wid: rg.w * (0.9 + rnd() * 0.2), dly: rg.dly + rnd() * 0.08,
          rel: peel ? fl.peelT + rnd() * 0.4 : fl.relT + ri * 0.03 + rnd() * 0.14, soft: peel, qAz: new T.Quaternion().setFromAxisAngle(new T.Vector3(0, 1, 0), az) });
      }
    });
  });
  var n = slots.length, geo = bloomPetalGeo(T, hd), aP = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4), aT = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4);
  aP.setUsage(T.DynamicDrawUsage); aT.setUsage(T.DynamicDrawUsage); geo.setAttribute('aP', aP); geo.setAttribute('aT', aT);
  var mat = bloomPetalMat(st, clip), mesh = new T.InstancedMesh(geo, mat, n);
  mesh.instanceMatrix.setUsage(T.DynamicDrawUsage); mesh.frustumCulled = false; mesh.receiveShadow = true; mesh.renderOrder = order;
  var PA = aP.array, TA = aT.array;
  slots.forEach(function (sl, i) {
    var tint = flowers[sl.f].tint, mixw = 0.16 * sl.rg.inner; // inner petals a touch deeper
    PA[i * 4 + 1] = sl.rg.el; PA[i * 4 + 2] = sl.rg.k; PA[i * 4 + 3] = sl.wid;
    TA[i * 4] = tint.r * (1 - mixw); TA[i * 4 + 1] = tint.g * (1 - mixw); TA[i * 4 + 2] = tint.b * (1 - mixw); TA[i * 4 + 3] = sl.rg.inner;
  });
  // Free petals: state arrays.
  var mode = new Uint8Array(n), px = new Float32Array(n), py = new Float32Array(n), pz = new Float32Array(n), vx = new Float32Array(n), vy = new Float32Array(n), vz = new Float32Array(n);
  var qx = new Float32Array(n), qy = new Float32Array(n), qz = new Float32Array(n), qw = new Float32Array(n), sax = new Float32Array(n), say = new Float32Array(n), saz = new Float32Array(n);
  var spin = new Float32Array(n), age = new Float32Array(n), life = new Float32Array(n), scl = new Float32Array(n), opn = new Float32Array(n), yaw = new Float32Array(n);
  var m4 = new T.Matrix4(), qa = new T.Quaternion(), qb = new T.Quaternion(), qc = new T.Quaternion(), v3 = new T.Vector3(), sv = new T.Vector3(), tv = new T.Vector3(), UP = new T.Vector3(0, 1, 0);
  mesh.instanceMatrix.array.fill(0);
  // Centres: a dome and a ring of stamens per flower, one instanced mesh of spheres.
  var NS = hd ? 7 : 5, cmesh = new T.InstancedMesh(new T.SphereGeometry(1, 10, 7), new T.MeshStandardMaterial({ color: '#d9890f', emissive: '#ff9a14', emissiveIntensity: 0.85, roughness: 0.5, clippingPlanes: clip }), flowers.length * (1 + NS));
  cmesh.instanceMatrix.setUsage(T.DynamicDrawUsage); cmesh.frustumCulled = false; cmesh.renderOrder = order; cmesh.instanceMatrix.array.fill(0);
  function release(i, fl, t) {
    var sl = slots[i], soft = sl.soft, wd = fl.size * sl.len * fl.vis;
    qa.multiplyQuaternions(fl.q, sl.qAz); v3.set(1, 0, 0).applyQuaternion(qa);
    mode[i] = 1; age[i] = 0; life[i] = Math.min((soft ? 1.5 + rnd() * 0.5 : 1.0 + rnd() * 0.55) * (fl.lifeK || 1), Math.max(0.3, (fl.endT || 99) - t)); scl[i] = wd * (0.42 + 0.58 * sl.open);
    px[i] = fl.x + v3.x * wd * 0.45; py[i] = fl.y + v3.y * wd * 0.45 + 0.05; pz[i] = fl.z + v3.z * wd * 0.45;
    var out = soft ? 0.3 + rnd() * 0.5 : 0.8 + rnd() * 1.3;
    vx[i] = v3.x * out; vy[i] = soft ? 0.3 + rnd() * 0.5 : 0.7 + rnd() * 1.2; vz[i] = v3.z * out;
    qx[i] = qa.x; qy[i] = qa.y; qz[i] = qa.z; qw[i] = qa.w;
    var ax = rnd() - 0.5, ay = rnd() - 0.5, az = rnd() - 0.5, al = Math.hypot(ax, ay, az) || 1;
    sax[i] = ax / al; say[i] = ay / al; saz[i] = az / al; spin[i] = (soft ? 1.5 : 3) + rnd() * 4; opn[i] = sl.open; yaw[i] = rnd() * 6.28;
  }
  // A free petal shrinks to nothing as it nears a place to keep clear, and is pushed away from it, so no petal
  // lies over another die.
  function clearFade(x, z) {
    var g = 1, k, c, d;
    for (k = 0; k < clear.length; k++) { c = clear[k]; d = Math.hypot(x - c.x, z - c.z); g *= smooth(c.r * 0.95, c.r * 1.4, d); }
    return g;
  }
  function clearPush(i, dt) {
    for (var k = 0; k < clear.length; k++) {
      var c = clear[k], dx = px[i] - c.x, dz = pz[i] - c.z, R = c.r * 1.9, d = Math.hypot(dx, dz) || 1e-3;
      if (d < R) { var w = 1 - d / R; w *= w; vx[i] += dx / d * w * 40 * dt; vz[i] += dz / d * w * 40 * dt; }
    }
  }
  function update(t, dt, wind) {
    var i, sl, fl;
    for (i = 0; i < n; i++) {
      sl = slots[i]; fl = flowers[sl.f];
      var o = i * 4;
      if (mode[i] === 0) {
        var tk = fl.tK || 1; sl.open = out3(clamp01((t - fl.tO - sl.dly * tk) / (0.7 * tk)));
        if (t >= sl.rel && fl.vis > 0.05 && sl.open > 0.3) release(i, fl, t);
        else {
          var sc = fl.size * sl.len * fl.vis * (0.42 + 0.58 * sl.open);
          if (sc < 0.002) { PA[o] = 0; m4.makeScale(0, 0, 0); mesh.setMatrixAt(i, m4); continue; }
          qa.multiplyQuaternions(fl.q, sl.qAz); sv.set(sc, sc, sc); v3.set(fl.x, fl.y, fl.z);
          m4.compose(v3, qa, sv); mesh.setMatrixAt(i, m4); PA[o] = sl.open; continue;
        }
      }
      if (mode[i] === 3) { continue; }
      // A free petal: blown on the wind, falling slowly, tumbling; it lands, lies, and fades.
      age[i] += dt; var k = age[i] / life[i];
      if (k >= 1) { mode[i] = 3; m4.makeScale(0, 0, 0); mesh.setMatrixAt(i, m4); continue; }
      if (mode[i] === 1) {
        vx[i] += (wind.x * wind.k - vx[i] * 0.9 + Math.sin(age[i] * 5 + i) * 0.7) * dt; vz[i] += (wind.z * wind.k - vz[i] * 0.9 + Math.cos(age[i] * 4.3 + i * 1.7) * 0.7) * dt;
        clearPush(i, dt);
        vy[i] += (-1.15 - vy[i]) * Math.min(1, 2.4 * dt);
        px[i] += vx[i] * dt; py[i] += vy[i] * dt; pz[i] += vz[i] * dt;
        var h = spin[i] * dt * 0.5, sh = Math.sin(h);
        qa.set(sax[i] * sh, say[i] * sh, saz[i] * sh, Math.cos(h)); qb.set(qx[i], qy[i], qz[i], qw[i]).multiply(qa); qx[i] = qb.x; qy[i] = qb.y; qz[i] = qb.z; qw[i] = qb.w;
        opn[i] += (0.5 - opn[i]) * Math.min(1, 3 * dt);
        if (py[i] <= 0.03) { py[i] = 0.03; mode[i] = 2; vx[i] *= 0.3; vz[i] *= 0.3; }
      } else {
        px[i] += vx[i] * dt; pz[i] += vz[i] * dt; vx[i] *= 0.9; vz[i] *= 0.9;
        qb.set(qx[i], qy[i], qz[i], qw[i]); qc.setFromAxisAngle(UP, yaw[i]); qb.slerp(qc, 1 - Math.exp(-6 * dt)); qx[i] = qb.x; qy[i] = qb.y; qz[i] = qb.z; qw[i] = qb.w;
      }
      var fade = 1 - smooth(0.55, 1, k), gone = clearFade(px[i], pz[i]);
      var s2 = scl[i] * fade * gone;
      qa.set(qx[i], qy[i], qz[i], qw[i]); sv.set(s2, s2, s2); v3.set(px[i], py[i], pz[i]);
      m4.compose(v3, qa, sv); mesh.setMatrixAt(i, m4); PA[o] = opn[i];
    }
    // centres
    var ci = 0, c, a;
    for (f = 0; f < flowers.length; f++) {
      fl = flowers[f];
      var sz = fl.size * (fl.vis > 0.02 ? fl.vis : 0), op = clamp01((t - fl.tO) / 0.8);
      qa.copy(fl.q);
      v3.set(fl.x, fl.y + sz * 0.07, fl.z); sv.set(sz * 0.2, sz * 0.12 * (0.5 + 0.5 * op), sz * 0.2); m4.compose(v3, qa, sv); cmesh.setMatrixAt(ci++, m4);
      for (c = 0; c < NS; c++) {
        a = c / NS * 6.2832 + fl.seed * 6;
        v3.set(Math.cos(a) * sz * 0.17, sz * 0.13, Math.sin(a) * sz * 0.17).applyQuaternion(qa).add(tv.set(fl.x, fl.y, fl.z));
        var sr = sz * 0.034 * smooth(0.3, 0.8, op); sv.set(sr, sr, sr); m4.compose(v3, qa, sv); cmesh.setMatrixAt(ci++, m4);
      }
    }
    mesh.instanceMatrix.needsUpdate = true; aP.needsUpdate = true; cmesh.instanceMatrix.needsUpdate = true;
  }
  return { mesh: mesh, cmesh: cmesh, mat: mat, update: update, count: n };
}

// ---------- The page under the garden ----------
// A green shade round the die so the vines and the light read on white, a damp mossy stain that spreads,
// roots crawling out ahead of it, and rings of light thrown out as the vines burst and the flowers open.
// Premultiplied, so it darkens and lights in one draw. Every place to keep clear is cut out of it.
var BLOOM_FLOOR_VS = 'varying vec2 vP; void main(){ vP = position.xy; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }';
var BLOOM_FLOOR_FS = [
  'uniform float uTime, uStain, uRoots, uShade, uPool, uFade, uS; uniform vec4 uRing; uniform vec3 uClr[12]; varying vec2 vP;',
  BLOOM_NOISE,
  'vec4 blOver(vec4 top, vec4 bot){ return top + bot * (1.0 - top.a); }',
  'void main(){',
  '  vec2 w = vec2(vP.x, -vP.y); float r = length(w), rs = r / uS, an = atan(w.y, w.x);',
  '  float hole = 1.0;',
  '  for (int i = 0; i < 12; i++) { vec3 c = uClr[i]; if (c.z > 0.0) hole *= smoothstep(c.z * 0.8, c.z * 1.0, length(w - c.xy)); }',
  '  float sh = uShade * exp(-rs * rs * 0.14);',
  '  vec4 res = vec4(vec3(0.01, 0.05, 0.015) * sh, sh);',
  '  float edgeN = blf(vec2(an * 2.2, uTime * 0.2)) * 0.6 + blf(w * 0.9) * 0.4;',
  '  float Rst = uStain * (2.7 + 1.5 * edgeN);',
  '  float inside = smoothstep(Rst, Rst - 0.8, rs);',
  '  float a1 = inside * 0.2;',
  '  res = blOver(vec4(vec3(0.03, 0.13, 0.03) * a1, a1), res);',
  '  float wa = an / 6.2832 * 13.0 + blf(vec2(rs * 0.55, an * 1.7)) * 3.0;',
  '  float tl = abs(fract(wa) - 0.5), wid = 0.07 * (1.0 - clamp((rs - 1.1) / max(Rst - 1.0, 0.5), 0.0, 1.0)) + 0.012;',
  '  float line = smoothstep(wid, wid * 0.35, tl);',
  '  float wb = an / 6.2832 * 29.0 + blf(vec2(rs * 0.9 + 3.0, an * 2.3)) * 4.0, tb = abs(fract(wb) - 0.5), wdb = 0.05 * (1.0 - clamp((rs - 1.8) / max(Rst - 1.6, 0.5), 0.0, 1.0)) + 0.008;',
  '  line = max(line, 0.7 * smoothstep(wdb, wdb * 0.35, tb) * smoothstep(1.8, 2.3, rs));',
  '  float reach = smoothstep(Rst + 0.3, Rst - 0.6, rs) * smoothstep(1.0, 1.4, rs);',
  '  float ra = clamp(line * reach * uRoots, 0.0, 1.0) * 0.55;',
  '  res = blOver(vec4(vec3(0.09, 0.14, 0.04) * ra, ra), res);',
  '  float w1 = 0.16 + 0.04 * uRing.x, w2 = 0.16 + 0.04 * uRing.z;',
  '  float r1 = uRing.y * exp(-pow((rs - uRing.x) / w1, 2.0)), r2 = uRing.w * exp(-pow((rs - uRing.z) / w2, 2.0));',
  '  res = blOver(vec4(vec3(0.02, 0.1, 0.02) * r1 * 0.16, r1 * 0.16), res);',
  '  res = blOver(vec4(vec3(1.0, 0.7, 0.2) * r2 * 0.06, 0.0), res);',
  '  gl_FragColor = res * (hole * uFade);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
function bloomFloor(st, at, s, uS, clear, order, span) {
  var T = st.T, arr = [], i;
  for (i = 0; i < 12; i++) arr.push(i < clear.length ? new T.Vector3(clear[i].x - at.x, clear[i].z - at.z, clear[i].r) : new T.Vector3(0, 0, 0));
  var uni = { uTime: { value: 0 }, uStain: { value: 0 }, uRoots: { value: 0 }, uShade: { value: 0 }, uPool: { value: 0 }, uFade: { value: 1 }, uS: { value: uS }, uRing: { value: new T.Vector4(0, 0, 0, 0) }, uClr: { value: arr } };
  var mat = new T.ShaderMaterial({ vertexShader: BLOOM_FLOOR_VS, fragmentShader: BLOOM_FLOOR_FS, uniforms: uni, transparent: true, depthWrite: false,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor });
  var m = new T.Mesh(new T.PlaneGeometry(span, span), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.008, at.z); m.renderOrder = order + 1; m.frustumCulled = false;
  m.userData.uni = uni;
  return m;
}

// ---------- Placing things on the stems ----------
// Leaves sit at the nodes, once the growing front has passed them. A leaf unfurls over the next quarter of
// the vine's length (and furls again as the front draws back), points away from the die, and is left out
// where it would reach into a place to keep clear.
function bloomPlaceLeaves(leaves, LV, vines, fe, cx, cz, clear, t) {
  for (var i = 0; i < LV.length; i++) {
    var L = LV[i], Pv = vines.P[L.v], Tv = vines.Tg[L.v], lt = clamp01((fe[L.v] - L.u) / 0.24);
    if (lt <= 0.002) { leaves.hide(i); continue; }
    var kk = L.k * 3, px = Pv[kk], py = Pv[kk + 1], pz = Pv[kk + 2], tx = Tv[kk], ty = Tv[kk + 1], tz = Tv[kk + 2];
    var ox = px - cx, oz = pz - cz, ol = Math.hypot(ox, oz) || 1; ox /= ol; oz /= ol;
    var sx = tz, sz = -tx, sl = Math.hypot(sx, sz);
    if (sl < 0.25) { sx = -oz; sz = ox; sl = 1; } sx /= sl; sz /= sl;
    var dx = ox * 0.62 + sx * L.sg * 0.5 + tx * 0.32, dy = Math.min(0.7, ty * 0.32 + 0.28), dz = oz * 0.62 + sz * L.sg * 0.5 + tz * 0.32;
    var dl = Math.sqrt(dx * dx + dy * dy + dz * dz); dx /= dl; dy /= dl; dz /= dl;
    var len = L.len * out3(Math.min(1, lt * 1.7)), ex = px + dx * len, ez = pz + dz * len;
    for (var c = 0; c < clear.length; c++) { var q = clear[c], a = ex - q.x, b = ez - q.z; if (a * a + b * b < q.r * q.r) { len = 0; break; } }
    if (len <= 0.001) { leaves.hide(i); continue; }
    leaves.set(i, px, py, pz, dx, dy, dz, len, L.wk, L.roll, smooth(0.12, 1, lt), L.seed, L.tint);
  }
  leaves.commit(t);
}
// A flower sits on the end of its vine, lifted a little, and shows as the vine's front arrives.
function bloomPlaceFlowers(FL, vines, fe) {
  for (var i = 0; i < FL.length; i++) {
    var f = FL[i], Pv = vines.P[f.v], e = vines.SEG * 3;
    f.x = Pv[e]; f.y = Pv[e + 1] + f.size * 0.1; f.z = Pv[e + 2]; f.vis = smooth(0.88, 0.995, fe[f.v]);
  }
}
// The gust blows along the direction that keeps furthest from every place to keep clear, leaning to the right
// and toward the viewer when nothing decides it.
function bloomGust(cx, cz, clear) {
  var best = -9, ba = 0.43, k, a, i, sc, m = 24;
  if (clear.length) for (k = 0; k < m; k++) {
    a = 0.43 + k / m * 6.2832; sc = 9;
    for (i = 0; i < clear.length; i++) {
      var c = clear[i], rx = c.x - cx, rz = c.z - cz, dl = Math.hypot(rx, rz) || 1, d = a - Math.atan2(rz, rx), da = Math.abs(Math.atan2(Math.sin(d), Math.cos(d)));
      sc = Math.min(sc, da - Math.asin(Math.min(1, c.r * 1.8 / dl)));
    }
    sc += 0.05 * Math.cos(a - 0.43);
    if (sc > best) { best = sc; ba = a; }
  }
  return { x: Math.cos(ba) * 3.27, z: Math.sin(ba) * 3.27, k: 0 };
}
// One flower's record: where its stem's path ends fixes which way it leans (away from `lean`, a point), and
// a random turn about its own axis staggers the petals of neighbours.
function bloomFlower(T, rnd, vines, v, size, tilt, lean, tintIx, tO, relT, peelT) {
  var up = new T.Vector3(0, 1, 0), S = vines.S[v], e = vines.SEG * 3, dx = S[e] - lean.x, dz = S[e + 2] - lean.z, dl = Math.hypot(dx, dz) || 1;
  var q = new T.Quaternion().setFromUnitVectors(up, new T.Vector3(dx / dl * tilt, 1, dz / dl * tilt).normalize());
  q.multiply(new T.Quaternion().setFromAxisAngle(up, rnd() * 6.28));
  return { v: v, size: size, tint: new T.Color(BLOOM.tints[tintIx % BLOOM.tints.length]), q: q, tO: tO, relT: relT, peelT: peelT, x: 0, y: -9, z: 0, vis: 0, seed: rnd() };
}
function bloomLeafList(cfg, SEG, rnd, s, LV) {
  cfg.forEach(function (c, vi) {
    c.lf.forEach(function (jn, q) {
      var u = (jn + 0.55) / BLOOM.nodes;
      LV.push({ v: vi, k: Math.round(u * SEG), u: u, sg: q % 2 ? 1 : -1, len: s * c.lsz * (1 - 0.42 * u) * (0.85 + rnd() * 0.3), wk: 0.9 + rnd() * 0.25, roll: (rnd() - 0.5) * 0.7, seed: rnd(), tint: rnd() });
    });
  });
}

// ---------- The effect ----------
// 0.0  the 20 kindles spring gold; roots crawl over the page and a green shade gathers
// 0.2  vines burst up round the die, leaves unfurling behind the growing tips; leaf litter flies
// 1.0  they coil up over its shoulders while runners creep across the page and arches reach out over it
// 1.7  buds at the tips open into flowers; at 2.0 the pollen bursts, a ring of gold runs over the page
//      and the die glows
// 2.7  the garden sways in full bloom, petals drifting
// 3.2  a gust strips the petals away; the vines draw back and sink into the page
// 4.6  gone: the die as it landed, the 20 lit
function fxBloom(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), rnd = mulberry(7321), numCol = bloomNumeral(th), END = 4.6, t = 0, Q = hd ? 1 : 0.5;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), cx = p0.x, cz = p0.z, yc = p0.y, at = new T.Vector3(cx, 0, cz);
  var clear = ((o && o.clear) || []).map(function (c) { return { x: c.x, z: c.z, r: c.r }; });
  // The vines keep a flower's width further off than the leaves do, so a flower head never opens over another die.
  var vc = clear.map(function (c) { return { x: c.x, z: c.z, r: c.r + s * 0.7 }; });
  var clip = [new T.Plane(new T.Vector3(0, 1, 0), 0)], centre = { x: cx, z: cz };
  var SEG = hd ? 84 : 54, SID = hd ? 10 : 7, NC = hd ? 5 : 4, NFo = hd ? 3 : 2, NR = hd ? 6 : 4, n = NC + NFo + NR;
  var vines = bloomVines(st, n, SEG, SID, clip, hd, 0), cfg = [], FL = [], LV = [], v = 0, i;

  // The vines: a ring that coils up the die, arches between them, and runners along the page.
  for (i = 0; i < NC; i++, v++) {
    var phi0 = i / NC * 6.2832 + 0.4 + (rnd() - 0.5) * 0.15;
    bloomPath(bloomCoilPts(cx, yc, cz, s, phi0, 3.1 + rnd() * 0.4, 1.4 + rnd() * 0.2, 1.12 + rnd() * 0.03), SEG, vines.S[v]);
    cfg.push({ d: 0.18 + 0.045 * i, dur: 1.4, rb: s * 0.17, sw: 0.5, lf: [1, 2, 3, 4, 5, 6, 7], lsz: 0.75 });
    FL.push(bloomFlower(T, rnd, vines, v, s * 0.4, 0.35, centre, FL.length * 5 + 1, 1.7 + 0.04 * i, 3.0 + rnd() * 0.25, 2.7 + rnd() * 0.3));
  }
  for (i = 0; i < NFo; i++, v++) {
    var phF = (i + 0.35) / NFo * 6.2832 + 0.4 + 3.1416 / NC, reach = clamp01((bloomReach(cx, cz, phF, s * 3.7, s * 2.6, vc) - s * 2.1) / (s * 1.6));
    bloomPath(bloomFountPts(cx, cz, s, phF, 1.0, reach), SEG, vines.S[v]);
    cfg.push({ d: 0.3 + 0.08 * i, dur: 1.55, rb: s * 0.15, sw: 1, lf: [1, 2, 3, 4, 5, 6], lsz: 0.72 });
    FL.push(bloomFlower(T, rnd, vines, v, s * 0.58, 0.3, centre, FL.length * 5 + 1, 1.85 + 0.05 * i, 3.05 + rnd() * 0.25, 2.8 + rnd() * 0.3));
  }
  for (i = 0; i < NR; i++, v++) {
    var ps = (i + 0.5) / NR * 6.2832 + 0.15, L = bloomReach(cx, cz, ps, s * (3.0 + rnd() * 1.4), s * 1.9, vc);
    bloomPath(bloomRunPts(cx, cz, s, ps, L, i % 2 ? 1 : -1), SEG, vines.S[v]);
    cfg.push({ d: 0.5 + 0.07 * i, dur: 1.3, rb: s * 0.12, sw: 1, lf: [1, 3, 4, 5, 6], lsz: 0.6 });
    if (i % 3 !== 2) FL.push(bloomFlower(T, rnd, vines, v, s * 0.45, 0.15, centre, FL.length * 5 + 1, 1.95 + 0.06 * i, 3.1 + rnd() * 0.2, 2.9 + rnd() * 0.3));
  }
  bloomLeafList(cfg, SEG, rnd, s, LV);
  FL.forEach(function (f) { f.endT = END - 0.05; });
  var leaves = bloomLeaves(st, LV.length, hd, clip, 0), blooms = bloomFlowers(st, FL, hd, clip, 0, clear, rnd);
  var floor = bloomFloor(st, at, s, s, clear, 0, s * 26), fu = floor.userData.uni;
  var core = glow(st, spriteTex(T), BLOOM.warm, s * 4), star = glow(st, starTex(T), '#fff2b0', s * 10);
  core.renderOrder = star.renderOrder = 7;
  var light = hd ? new T.PointLight(BLOOM.warm, 0, s * 15, 1.6) : null;
  var own = [floor, vines.mesh, leaves.mesh, blooms.mesh, blooms.cmesh, core, star];
  st.scene.add(floor, vines.mesh, leaves.mesh, blooms.mesh, blooms.cmesh, core, star);
  if (light) st.scene.add(light);

  var fe = new Float32Array(n), wind = bloomGust(cx, cz, clear), done = {}, gf = glowFace(die, fi), tp = worldOf(die, die.faces[fi].c), topY = tp.y;
  function puff(nAdd, nDark, speed, maxLife) {
    function spawn(i, r) {
      var fl = FL[i % FL.length], a = r() * 6.28, sp = (0.3 + r() * 1.6) * speed;
      return { p: [fl.x, fl.y + fl.size * 0.2, fl.z], v: [Math.cos(a) * sp, 0.8 + r() * 2.2, Math.sin(a) * sp], delay: r() * 0.55 };
    }
    st.fx.push(spray(st, { n: Math.round(nAdd * Q), colors: ['#ffe27a', '#fff4b8', '#ffffff', '#ffc83a'], size: s * 0.075, life: [1.0, maxLife || 2.0], drag: 0.8, g: -0.35, spawn: spawn }));
    st.fx.push(spray(st, { n: Math.round(nDark * Q), colors: ['#e0a21a', '#f2c43a', '#d98a12'], size: s * 0.055, life: [1.0, maxLife || 2.0], drag: 0.8, g: -0.35, blend: 'normal', alpha: 0.9, spawn: spawn }));
  }

  return { update: function (dt) {
    t += dt;
    var i, c;
    // The fronts: out fast, then slowing as they coil; and, from 3.25, drawing back.
    for (i = 0; i < n; i++) {
      c = cfg[i];
      var x = clamp01((t - c.d) / c.dur), gr = 1 - Math.pow(1 - x, 2.3);
      fe[i] = gr * (1 - inout((t - 3.45 - 0.02 * i) / 0.8));
    }
    var sink = s * 0.9 * smooth(3.8, 4.5, t), grown = smooth(0.4, 1.4, t);
    for (i = 0; i < n; i++) { c = cfg[i]; vines.put(i, fe[i], 0.8 + 0.2 * grown, t, s * 0.05 * c.sw * smooth(0.6, 1.6, t), sink, vc, c.rb); }
    vines.commit(t, env4(t, 1.6, 2.2, 3.0, 3.9) * 0.55);
    bloomPlaceLeaves(leaves, LV, vines, fe, cx, cz, clear, t);
    bloomPlaceFlowers(FL, vines, fe);
    wind.k = smooth(3.1, 3.5, t) * (1 - smooth(4.2, 4.6, t));
    blooms.mat.userData.uni.uGlow.value = env4(t, 1.8, 2.4, 3.0, 3.6) * 0.8;
    blooms.update(t, dt, wind);

    if (!done.litter && t >= 0.2) {
      done.litter = true;
      st.fx.push(spray(st, { n: Math.round(70 * Q), colors: ['#4c8a22', '#2f6a1a', '#7a5a2a', '#8fc63a'], size: s * 0.09, life: [0.7, 1.5], drag: 0.5, g: 8, blend: 'normal', alpha: 1,
        spawn: function (i, r) { var a = r() * 6.28, rr = (1.3 + r() * 0.9) * s, sp = 0.3 + r() * 1.2; return { p: [cx + Math.cos(a) * rr, 0.05, cz + Math.sin(a) * rr], v: [Math.cos(a) * sp, 2.5 + r() * 4.5, Math.sin(a) * sp], delay: r() * 0.45 }; } }));
    }
    if (!done.puff && t >= 2.0) { done.puff = true; puff(260, 120, 1); }
    if (!done.puff2 && t >= 2.8) { done.puff2 = true; puff(90, 40, 0.5, 1.5); }

    // The page.
    var k1 = clamp01((t - 0.05) / 1.0), k2 = clamp01((t - 1.95) / 1.2);
    fu.uTime.value = t;
    fu.uShade.value = 0.36 * smooth(0.1, 0.9, t);
    fu.uStain.value = smooth(0.05, 1.3, t);
    fu.uRoots.value = smooth(0.05, 0.9, t) * (1 - smooth(2.8, 3.8, t));
    fu.uPool.value = env4(t, 1.7, 2.3, 3.0, 4.0);
    fu.uRing.value.set(1.5 + 3.5 * out3(k1), 0.8 * (1 - k1) * (t > 0.05 ? 1 : 0), 1.5 + 4 * out3(k2), t >= 1.95 ? 1.0 * (1 - k2) * (1 - k2) : 0);
    fu.uFade.value = 1 - smooth(3.7, 4.5, t);

    // The die: the 20 kindles, then the whole die warms to gold as the flowers open.
    var env = env4(t, 1.5, 2.1, 3.0, 4.3);
    if (t < 0.95) { var p = clamp01((t - 0.08) / 0.75); drawBurn(gf.g, gf.lay, p, numCol); gf.commit(1.6); }
    die.faces[fi].mat.emissiveIntensity = 1.6 + 0.4 * env4(t, 1.8, 2.2, 3.0, 4.0);
    tintDie(die, BLOOM.gold, 0.14 * env);
    core.position.set(cx, yc, cz); core.material.opacity = 0.2 * env; core.scale.setScalar(s * (3 + 1.5 * env));
    var sk = (t - 1.95) / 0.55;
    star.position.set(cx, yc, cz); star.material.opacity = sk >= 0 && sk < 1 ? 0.8 * (1 - sk) * (1 - sk) : 0; star.material.rotation = sk * 0.5; star.scale.setScalar(s * (6 + 8 * out3(clamp01(sk))));
    if (light) { light.position.set(cx, topY + s * 1.6, cz); light.intensity = 8 * env4(t, 1.5, 2.1, 3.1, 4.2) * (0.92 + 0.08 * Math.sin(t * 11)); }

    if (t >= END) {
      drop(st, own);
      leaves.mesh.dispose(); blooms.mesh.dispose(); blooms.cmesh.dispose();
      if (light) { st.scene.remove(light); light.dispose(); }
      g.position.copy(p0); g.quaternion.copy(q0);
      gf.clear(); glowNumeral(die, fi, numCol); die.faces[fi].mat.emissiveIntensity = 1.6; tintDie(die, th.burn, 0);
      return false;
    }
    return true;
  } };
}

// ---------- The echo ----------
// One vine runs across the page from the big die to this one, and a single flower opens beside it, the
// grand's garden in small. With no big die to run from, a sprout rises beside the die instead.
function fxBloomEcho(st, die, fi, th, e) {
  var T = st.T, s = die.size, hd = !(st.q && st.q.detail === 'basic'), rnd = mulberry(5521 + ((e && e.order) || 0) * 17), t = 0, ORD = 60, numCol = bloomNumeral(th);
  var p0 = die.group.position.clone(), cx = p0.x, cz = p0.z, from = e && e.from ? e.from : null, clear = [], j;
  st.dice.forEach(function (d) { if (d !== die && d !== from) { var p = d.group.position; clear.push({ x: p.x, z: p.z, r: d.size * 1.5 }); } });
  function blocked(x, z, pad) { for (var k = 0; k < clear.length; k++) { var c = clear[k], a = x - c.x, b = z - c.z; if (Math.hypot(a, b) < c.r + pad) return true; } return false; }
  // The flower's place: beside the die, on the side facing the big die (or the middle of the page).
  var dx = (from ? from.group.position.x : 0) - cx, dz = (from ? from.group.position.z : 0) - cz, dl = Math.hypot(dx, dz);
  if (dl < 0.01) { dx = 1; dz = 0; dl = 1; } dx /= dl; dz /= dl;
  var a0 = Math.atan2(dz, dx), fx = cx, fz = cz, tries = [0, 0.7, -0.7, 1.4, -1.4, 2.1, -2.1, 3.1416];
  for (j = 0; j < tries.length; j++) { fx = cx + Math.cos(a0 + tries[j]) * s * 1.65; fz = cz + Math.sin(a0 + tries[j]) * s * 1.65; if (!blocked(fx, fz, s * 0.4)) break; }
  var ax = 0, az = 0, run = 0;
  if (from) { ax = from.group.position.x - dx * s * 1.25; az = from.group.position.z - dz * s * 1.25; run = Math.hypot(fx - ax, fz - az); }
  var long = from && run > s * 1.2, SEG = hd ? 56 : 36, SID = hd ? 8 : 6, pts = [], ph = rnd() * 6;
  if (long) {
    var vx = fx - ax, vz = fz - az, sdx = -vz / run, sdz = vx / run;
    pts.push(ax, -0.25 * s, az, ax, 0.06 * s, az);
    for (j = 1; j <= 4; j++) { var f = j / 5, m = 0.35 * s * Math.sin(f * 3.6 + ph); pts.push(ax + vx * f + sdx * m, (0.07 + 0.03 * Math.sin(f * 8)) * s, az + vz * f + sdz * m); }
    pts.push(fx, 0.12 * s, fz, fx, 0.55 * s, fz);
  } else {
    var sx = -dz, sz = dx;
    pts.push(fx, -0.5 * s, fz, fx, 0.05 * s, fz, fx + sx * 0.12 * s, 0.3 * s, fz + sz * 0.12 * s, fx - sx * 0.1 * s, 0.5 * s, fz - sz * 0.1 * s, fx, 0.62 * s, fz);
  }
  var growDur = long ? 0.7 : 0.5, tO = long ? 0.5 : 0.35, relT = tO + (long ? 0.85 : 0.85), END = relT + 0.5, retT = relT + 0.05;
  var clip = [new T.Plane(new T.Vector3(0, 1, 0), 0)], vines = bloomVines(st, 1, SEG, SID, clip, hd, ORD);
  bloomPath(pts, SEG, vines.S[0]);
  var cfg = [{ lf: long ? (hd ? [1, 3, 5, 7] : [2, 5]) : [4, 6], lsz: 0.42 }], LV = [], FL = [], rb = s * (long ? 0.075 : 0.07);
  bloomLeafList(cfg, SEG, rnd, s, LV);
  var fl = bloomFlower(T, rnd, vines, 0, s * 0.36, 0.2, { x: cx, z: cz }, Math.floor(rnd() * 6), tO, relT, relT - 0.1);
  fl.tK = 0.65; fl.lifeK = 0.35; fl.endT = END - 0.05; FL.push(fl);
  var leaves = bloomLeaves(st, Math.max(1, LV.length), hd, clip, ORD), blooms = bloomFlowers(st, FL, hd, clip, ORD, clear.concat([{ x: cx, z: cz, r: s * 1.2 }]), rnd);
  var at = new T.Vector3(fx, 0, fz), floor = bloomFloor(st, at, s, s * 0.5, clear, ORD, s * 12), fu = floor.userData.uni;
  var own = [floor, vines.mesh, leaves.mesh, blooms.mesh, blooms.cmesh];
  st.scene.add(floor, vines.mesh, leaves.mesh, blooms.mesh, blooms.cmesh);
  var fe = new Float32Array(1), wind = { x: -dz * (dx < 0 ? -1 : 1) * 1.6, z: Math.abs(dx) * 1.6, k: 0 }, done = {}, gf = glowFace(die, fi);
  var tp = worldOf(die, die.faces[fi].c);

  return { update: function (dt) {
    t += dt;
    fe[0] = out3(t / growDur) * (1 - inout((t - retT) / 0.4));
    vines.put(0, fe[0], 0.9, t, s * 0.02, s * 0.5 * smooth(retT + 0.05, retT + 0.4, t), clear, rb);
    vines.commit(t, 0);
    bloomPlaceLeaves(leaves, LV, vines, fe, cx, cz, clear, t);
    bloomPlaceFlowers(FL, vines, fe);
    wind.k = smooth(relT - 0.05, relT + 0.1, t);
    blooms.mat.userData.uni.uGlow.value = env4(t, tO, tO + 0.3, relT, relT + 0.4) * 0.6;
    blooms.update(t, dt, wind);
    if (!done.puff && t >= tO + 0.25) {
      done.puff = true;
      var spawn = function (i, r) { var a = r() * 6.28, sp = 0.2 + r() * 1.0; return { p: [fl.x, fl.y + fl.size * 0.2, fl.z], v: [Math.cos(a) * sp, 0.6 + r() * 1.4, Math.sin(a) * sp], delay: r() * 0.3 }; };
      st.fx.push(spray(st, { n: hd ? 60 : 28, colors: ['#ffe27a', '#fff4b8', '#ffffff'], size: s * 0.06, life: [0.7, 1.3], drag: 0.8, g: -0.3, spawn: spawn }));
      st.fx.push(spray(st, { n: hd ? 28 : 12, colors: ['#e0a21a', '#f2c43a'], size: s * 0.05, life: [0.7, 1.3], drag: 0.8, g: -0.3, blend: 'normal', alpha: 0.9, spawn: spawn }));
    }
    var k2 = clamp01((t - tO) / 0.8), env = env4(t, tO, tO + 0.4, relT, END);
    fu.uTime.value = t; fu.uShade.value = 0.2 * smooth(0, 0.4, t); fu.uStain.value = smooth(0, 0.6, t); fu.uRoots.value = 0.5 * smooth(0, 0.5, t) * (1 - smooth(relT, END, t));
    fu.uPool.value = 0.8 * env; fu.uRing.value.set(0, 0, 1 + 3 * out3(k2), t >= tO ? 0.55 * (1 - k2) * (1 - k2) : 0); fu.uFade.value = 1 - smooth(END - 0.5, END - 0.05, t);
    if (t < 0.95) { drawBurn(gf.g, gf.lay, clamp01((t - tO + 0.1) / 0.5), numCol); gf.commit(1.6); }
    die.faces[fi].mat.emissiveIntensity = 1.6 + 0.5 * env;
    tintDie(die, BLOOM.gold, 0.12 * env);
    if (t >= END) {
      drop(st, own); leaves.mesh.dispose(); blooms.mesh.dispose(); blooms.cmesh.dispose();
      gf.clear(); glowNumeral(die, fi, numCol); die.faces[fi].mat.emissiveIntensity = 1.6; tintDie(die, th.burn, 0);
      return false;
    }
    return true;
  } };
}
registerCrit('bloom', { name: 'Wildbloom', kind: 'succ', what: 'Vines burst up out of the page round the die, coil over it and flower; the petals blow away, the vines sink back, and the 20 is left glowing, spring gold on a dark die.',
  fx: fxBloom, beat: 2.0, peak: 3.0, echo: fxBloomEcho, echoWhat: 'A single vine runs across the page to the die and one flower opens beside it.' });
