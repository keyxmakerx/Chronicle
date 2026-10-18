// ---------- Crit success: Dawnbreak ----------
// The page dims round the die under a storm-dark sky. The cloud tears open above it, ragged and gold-
// edged, and shafts of light break through: seen from straight above they are bright wedges fanning out
// across the page from the die, with dust drifting through them and catching the light. The die is lit
// gold and a halo, a ring of fine radial light with a band of glowing runes, draws itself in over the
// die, descends and hovers there. A pulse of light runs out along the shafts, then everything settles
// and the 20 is left lit gold.
//
// The sky is one shader on one sheet lying on the page: cloud, the opening and its gold rim, the shafts
// and the pool of light all in a single premultiplied draw. Light only adds, and on a white page adding
// gold vanishes, so the cloud wash does the dimming and the shafts cut it away and tint what is under
// them gold; that contrast is what carries the picture on a light page, and on a dark one the gold adds.
import {
  PX, canvas, clamp01, dimmer, drop, env4, glow, glowFace, glowLayer, hexA, mix, mulberry, out3,
  paintNumeral, registerCrit, shadeDie, smooth, spray, spriteTex, strokeW, tintDie, worldOf, share
} from '../engine.js';

var DAWN = { hot: '#fff3d2', gold: '#ffb92e', amber: '#f2a01e', rose: '#ff8a5c', back: '#4a2b08',
  deep: '#0f1220', body: '#262c46', lit: '#6a7494', num: '#f2a30f', numDeep: '#d98300' };

// ---------- Shared shader pieces ----------
// Value noise, and the shafts. The shafts are a function of the angle round the die (noise read round a
// circle, so it has no seam), turned slowly and morphing, so the wedges drift rather than sit still.
// The sky and the dust motes both read it, so a mote lights up exactly where a shaft passes.
var DAWN_NOISE = [
  'float dwh(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float dwn(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(dwh(i), dwh(i + vec2(1.0, 0.0)), u.x), mix(dwh(i + vec2(0.0, 1.0)), dwh(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float dwf(vec2 p){ float s = 0.0, a = 0.5, w = 0.0; for (int i = 0; i < DW_OCT; i++) { s += a * dwn(p); w += a; p = p * 2.03 + vec2(1.7, 9.2); a *= 0.5; } return s / w; }',
  'float dwa(float a, float b){ return abs(atan(sin(a - b), cos(a - b))); }',
  'float dw2(float x){ return x * x; }'
].join('\n');

var DAWN_RAYS = [
  'uniform float uT, uSpin, uReach, uRay, uS, uSurgeR, uSurgeA;',
  'float dwRay(vec2 p){',
  '  float r = length(p), an = atan(p.y, p.x + 1e-6);',
  '  vec2 d = vec2(cos(an + uSpin), sin(an + uSpin));',
  '  float a = dwn(d * 2.8 + vec2(uT * 0.06, 2.1)), b = dwn(d * 6.2 + vec2(-uT * 0.1, 7.7)), e = dwn(d * 4.1 + vec2(5.3, uT * 0.05)), c = dwn(d * 14.0 + vec2(uT * 0.15, 3.9));',
  // Strong shafts and fainter ones between, then finer streaks inside each.
  '  float v = a * 0.4 + b * 0.35 + e * 0.25;',
  '  float wedge = 0.06 + 0.3 * smoothstep(0.4, 0.5, v) + 0.64 * smoothstep(0.52, 0.6, v);',
  '  float fine = 0.45 + 0.55 * smoothstep(0.3, 0.75, c);',
  '  float reach = uReach * (0.8 + 0.4 * dwn(d * 2.3 + 9.0));',
  '  float rad = smoothstep(uS * 0.8, uS * 1.8, r) * (1.0 - smoothstep(reach * 0.3, reach, r));',
  '  float dust = 0.72 + 0.28 * dwn(vec2(d.x * 9.0 + r * 0.45 - uT * 1.8, d.y * 9.0));',
  '  float surge = 1.0 + uSurgeA * exp(-dw2((r - uSurgeR) / (uS * 2.2)));',
  '  return wedge * fine * rad * dust * surge * uRay; }'
].join('\n');

// ---------- The sky ----------
var DAWN_SKY_VS = 'varying vec2 vW; void main(){ vec4 w = modelMatrix * vec4(position, 1.0); vW = w.xz; gl_Position = projectionMatrix * viewMatrix * w; }';
var DAWN_SKY_FS = [
  'uniform float uWash, uOpen, uPool, uRw, uRingR, uRingA;',
  'uniform vec2 uC; uniform vec3 uClr[12]; uniform vec3 uDeep, uBody, uLit, uGold, uHot, uRose; varying vec2 vW;',
  DAWN_NOISE, DAWN_RAYS,
  'void main(){',
  '  vec2 p = vW - uC; float r = length(p);',
  // Cloud: big billows with a finer churn warped by them.
  '  float n = dwf(p * 0.2 + vec2(uT * 0.025, 2.3));',
  '  float m = dwf(p * 0.62 + vec2(-uT * 0.05, uT * 0.03) + n * 1.3);',
  '  float cl = n * 0.65 + m * 0.35;',
  // The wash ends in a ragged soft edge; the opening in it is ragged too, with a bright rim.
  '  float ext = 1.0 - smoothstep(uRw * 0.5, uRw, r + (cl - 0.5) * uRw * 0.5);',
  '  float op = max(uOpen, 0.05);',
  '  float rr = r + (cl - 0.5) * (op * 0.6 + uS * 0.6);',
  '  float hole = 1.0 - smoothstep(op * 0.7, op * 1.05, rr);',
  '  float edge = exp(-dw2((rr - op * 0.95) / (uS * 0.12 + op * 0.03))) * smoothstep(0.0, uS, uOpen) * (0.45 + 1.1 * m) * smoothstep(0.3, 0.95, uWash);',
  '  float ray = dwRay(p);',
  '  float ring = exp(-dw2((r - uRingR) / (uS * 0.13 + 0.025 * uRingR))) * uRingA;',
  // A clear patch round each other die: the wash thins away and the added light is held back.
  '  float keep = 1.0;',
  '  for (int i = 0; i < 12; i++) keep *= smoothstep(uClr[i].z * 0.7, uClr[i].z * 1.5, length(vW - uClr[i].xy));',
  '  float wa = uWash * ext * clamp(0.78 + (cl - 0.45) * 1.3, 0.4, 1.0) * 0.88;',
  '  wa *= (1.0 - 0.85 * hole) * (1.0 - 0.85 * clamp(ray, 0.0, 1.0)) * keep;',
  '  vec3 cc = mix(uDeep, uBody, smoothstep(0.2, 0.8, cl));',
  '  cc = mix(cc, uLit, smoothstep(0.55, 0.92, m) * 0.5);',
  // Cloud near the opening and in the shafts is sunlit.
  '  float warm = clamp(edge * 1.2 + ray * 0.6 + hole * 0.35, 0.0, 1.0);',
  '  cc = mix(cc, mix(uGold, uRose, 0.3 * (1.0 - edge)), warm * 0.7);',
  '  vec3 col = cc * wa; float a = wa;',
  '  float pool = uPool * exp(-r * r / (uS * uS * 9.0));',
  // Gold that covers a little, so the light reads on a white page, where adding light shows nothing.
  '  float ga = clamp(ray * 0.5 + pool * 0.42 + edge * 0.35 + ring * 0.5, 0.0, 0.8) * (0.3 + 0.7 * keep);',
  '  vec3 gc = mix(uGold, uHot, clamp(edge * 0.4, 0.0, 0.5));',
  '  gc = mix(gc, uRose, smoothstep(uReach * 0.45, uReach, r) * 0.4);',
  // Premultiplying in linear light makes a covering colour read paler than its alpha says; this keeps
  // the tint as strong as the alpha asks.
  '  col = col * (1.0 - ga) + gc * pow(ga, 2.2); a = a * (1.0 - ga) + ga;',
  // Added light stays well under 1 here: the bloom would carry anything brighter over the whole page.
  '  vec3 add = uHot * (ray * 0.14 + edge * 0.35 + pool * 0.04 + ring * 0.15) * (0.4 + 0.6 * keep);',
  '  col += add;',
  '  if (a + dot(add, vec3(1.0)) < 0.003) discard;',
  '  gl_FragColor = vec4(col, a);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'].join('\n');

// A premultiplied material: colour adds, alpha covers what is under it.
function dawnMat(st, vs, fs, uni, defs, extra) {
  if (defs.DW_OCT == null) defs.DW_OCT = 2;
  var T = st.T, o = { vertexShader: vs, fragmentShader: fs, uniforms: uni, defines: defs, transparent: true, depthWrite: false, side: T.DoubleSide,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor };
  for (var k in extra) o[k] = extra[k];
  return new T.ShaderMaterial(o);
}
function dawnCol(T, hex) { return { value: new T.Color(hex) }; }

// c: where the shafts come from on the page (the die as the camera sees it, in page coordinates);
// clr: the places to keep clear, as Vector3 (x, z, r).
function dawnSky(st, c, s, hd, clr) {
  var T = st.T, RW = s * 10.5, arr = [], i;
  for (i = 0; i < 12; i++) arr.push(clr[i] ? clr[i].clone() : new T.Vector3(1e5, 1e5, 1));
  var uni = { uT: { value: 0 }, uSpin: { value: 0 }, uReach: { value: s * 3 }, uRay: { value: 0 }, uS: { value: s }, uSurgeR: { value: 0 }, uSurgeA: { value: 0 },
    uWash: { value: 0 }, uOpen: { value: 0 }, uPool: { value: 0 }, uRw: { value: RW }, uRingR: { value: 0 }, uRingA: { value: 0 },
    uC: { value: new T.Vector2(c.x, c.z) }, uClr: { value: arr },
    uDeep: dawnCol(T, DAWN.deep), uBody: dawnCol(T, DAWN.body), uLit: dawnCol(T, DAWN.lit), uGold: dawnCol(T, DAWN.gold), uHot: dawnCol(T, DAWN.hot), uRose: dawnCol(T, DAWN.rose) };
  var mesh = new T.Mesh(new T.PlaneGeometry(RW * 2.2, RW * 2.2), dawnMat(st, DAWN_SKY_VS, DAWN_SKY_FS, uni, { DW_OCT: hd ? 4 : 2 }, {}));
  // Under everything else: the page's own shadows and the dice draw over it.
  mesh.rotation.x = -Math.PI / 2; mesh.position.set(c.x, 0.01, c.z); mesh.renderOrder = -3; mesh.frustumCulled = false; mesh.visible = false;
  return { mesh: mesh, uni: uni };
}

// ---------- The dust ----------
// Motes drifting round the die. Their brightness is read from the same shaft function as the sky, at
// the page point they appear over, so they are faint specks until a shaft crosses them and then flare.
var DAWN_MOTE_VS = [
  'attribute float aSize, aAl; uniform float uCamY, uK; uniform vec2 uC; varying float vA, vL;',
  DAWN_NOISE, DAWN_RAYS,
  'void main(){',
  '  vec4 w = modelMatrix * vec4(position, 1.0);',
  '  float m = uCamY / max(uCamY - w.y, 1.0);',
  '  float lit = clamp(dwRay(w.xz * m - uC), 0.0, 1.6);',
  '  vA = aAl * (0.2 + 1.1 * lit); vL = lit;',
  '  vec4 mv = viewMatrix * w; gl_Position = projectionMatrix * mv;',
  '  gl_PointSize = max(2.0, aSize * uK / -mv.z * (0.75 + 0.6 * lit));',
  '}'].join('\n');
var DAWN_MOTE_FS = [
  'uniform vec3 uHot, uGold; varying float vA, vL;',
  'void main(){',
  '  vec2 q = (gl_PointCoord - 0.5) * 2.0; float d2 = dot(q, q);',
  '  float core = exp(-d2 * 7.0), halo = exp(-d2 * 2.2) * 0.3;',
  // A faint cross on the bright ones, like a lit speck seen through a lens.
  '  float cross = (exp(-abs(q.x) * 16.0) * (1.0 - abs(q.y)) + exp(-abs(q.y) * 16.0) * (1.0 - abs(q.x))) * clamp(vL - 0.4, 0.0, 1.0) * 0.5;',
  '  float I = (core + halo + cross) * vA;',
  '  if (I < 0.004) discard;',
  '  gl_FragColor = vec4(mix(uHot, uGold, 0.35 * (1.0 - core)) * I, clamp(core * vA * 0.7 + halo * vA * 0.3, 0.0, 0.8));',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'].join('\n');

function dawnMotes(st, n, s, sky, rnd, clr, cx0, cz0) {
  var T = st.T, pos = new Float32Array(n * 3), sz = new Float32Array(n), al = new Float32Array(n);
  var geo = new T.BufferGeometry(), pa = new T.BufferAttribute(pos, 3), aa = new T.BufferAttribute(al, 1), U = sky.uni;
  pa.setUsage(T.DynamicDrawUsage); aa.setUsage(T.DynamicDrawUsage);
  geo.setAttribute('position', pa); geo.setAttribute('aSize', new T.BufferAttribute(sz, 1)); geo.setAttribute('aAl', aa);
  var uni = { uT: U.uT, uSpin: U.uSpin, uReach: U.uReach, uRay: U.uRay, uS: U.uS, uSurgeR: U.uSurgeR, uSurgeA: U.uSurgeA, uC: U.uC,
    uCamY: { value: st.cam.position.y }, uK: { value: 1 }, uHot: dawnCol(T, DAWN.hot), uGold: dawnCol(T, DAWN.gold) };
  var mesh = new T.Points(geo, dawnMat(st, DAWN_MOTE_VS, DAWN_MOTE_FS, uni, { DW_OCT: 2 }, {}));
  mesh.frustumCulled = false; mesh.renderOrder = 7; mesh.visible = false;
  var R = new Float32Array(n), A = new Float32Array(n), Y = new Float32Array(n), W = new Float32Array(n), V = new Float32Array(n),
    life = new Float32Array(n), age = new Float32Array(n), ph = new Float32Array(n), i;
  function clear(x, z) { for (var k = 0; k < clr.length; k++) if (Math.hypot(x - clr[k].x, z - clr[k].y) < clr[k].z * 1.5) return true; return false; }
  function spawn(i, cx, cz, initial) {
    for (var tr = 0; tr < 6; tr++) {
      R[i] = s * (1.3 + 8 * Math.sqrt(rnd())); A[i] = rnd() * 6.2832;
      if (!clear(cx + Math.cos(A[i]) * R[i], cz + Math.sin(A[i]) * R[i])) break;
    }
    Y[i] = s * (0.25 + rnd() * 2.6); W[i] = (rnd() - 0.3) * 0.18; V[i] = s * (0.06 + rnd() * 0.16);
    life[i] = 2.4 + rnd() * 2.4; age[i] = initial ? rnd() * life[i] : 0; ph[i] = rnd();
    sz[i] = s * (0.07 + rnd() * rnd() * 0.2);
  }
  for (i = 0; i < n; i++) spawn(i, cx0, cz0, true);
  return { mesh: mesh,
    // glob: overall level; pull: draw toward the halo's radius rt; out: thrown outward; swirl: extra turn.
    step: function (dt, t, cx, cz, glob, pull, rt, out, swirl) {
      uni.uK.value = PX * st.renderer.getPixelRatio() * st.dist;
      mesh.visible = glob > 0.003;
      for (var i = 0; i < n; i++) {
        age[i] += dt;
        if (age[i] >= life[i]) spawn(i, cx, cz, false);
        var k = age[i] / life[i];
        A[i] += (W[i] + swirl) * dt / (0.6 + R[i] / s * 0.15);
        R[i] += ((rt - R[i]) * pull + out * (0.3 + R[i] / s * 0.12)) * dt;
        Y[i] += V[i] * dt;
        var x = cx + Math.cos(A[i]) * R[i], z = cz + Math.sin(A[i]) * R[i];
        pos[i * 3] = x; pos[i * 3 + 1] = Y[i]; pos[i * 3 + 2] = z;
        al[i] = glob * smooth(0, 0.2, k) * (1 - smooth(0.7, 1, k)) * (0.75 + 0.25 * Math.sin(t * (2 + ph[i] * 3) + ph[i] * 6.28));
      }
      pa.needsUpdate = true; aa.needsUpdate = true;
    } };
}

// ---------- The halo ----------
// A ring of light lying flat over the die. A band between two fine rims holds a ring of runes, each one
// lighting as the ring draws itself round from the top; beyond the outer rim and inside the inner one,
// fine radial filaments, like the corona of a sun. The band is backed by a warm dark amber so the
// glyphs read on a white page; the rims carry gold that covers a little, so the ring reads there too.
var DAWN_HALO_VS = 'varying vec2 vP; void main(){ vP = position.xy; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }';
var DAWN_HALO_FS = [
  'uniform float uT, uR, uB, uForm, uFade, uGlint, uRot, uFlare, uCells, uDie;',
  'uniform vec3 uHot, uGold, uAmber, uBack; uniform sampler2D uRune; varying vec2 vP;',
  DAWN_NOISE,
  'void main(){',
  '  float r = length(vP); if (r < 0.002) discard;',
  '  float an = atan(vP.y, vP.x), TAU = 6.2831853;',
  '  vec2 d = vec2(cos(an), sin(an));',
  '  float t = (r - uR) / uB;',
  // The ring draws from the top both ways round and meets at the bottom.
  '  float ad = dwa(an, 1.5708), front = uForm * 4.1;',
  '  float vis = 1.0 - smoothstep(front - 0.14, front + 0.02, ad);',
  '  float head = exp(-dw2((ad - front + 0.02) / 0.09)) * step(0.001, uForm) * (1.0 - smoothstep(0.93, 1.0, uForm));',
  '  float inBand = 1.0 - smoothstep(0.9, 1.0, abs(t));',
  '  float rimO = exp(-dw2((t - 1.0) / 0.11)), rimI = exp(-dw2((t + 1.0) / 0.09)), rimX = exp(-dw2((t - 1.32) / 0.05));',
  '  float bead = pow(0.5 + 0.5 * cos(an * uCells * 2.0), 10.0);',
  // The runes: cell by cell round the band, each lit once the drawing has passed it, with a flare as it does.
  '  float cg = (an / TAU + uRot) * uCells, gi = floor(cg), fu = fract(cg);',
  '  vec2 guv = vec2((mod(gi, 24.0) + 0.1 + 0.8 * fu) / 24.0, 0.08 + 0.84 * clamp(t * 0.5 + 0.5, 0.0, 1.0));',
  '  float tx = texture2D(uRune, guv).r * inBand * step(abs(t), 0.84);',
  '  float ac = dwa((gi + 0.5) / uCells * TAU - uRot * TAU, 1.5708);',
  '  float lit = 1.0 - smoothstep(front - 0.7, front - 0.3, ac);',
  '  float flare = exp(-dw2((front - 0.5 - ac) / 0.3)) * lit;',
  '  float tw = 0.85 + 0.15 * sin(uT * 3.1 + dwh(vec2(gi, 3.0)) * 6.28);',
  '  float g = (smoothstep(0.35, 0.8, tx) + tx * 0.35) * (lit * tw + flare * 1.2);',
  '  float rim = (rimO + 0.8 * rimI + 0.5 * rimX) * vis;',
  '  float beadK = bead * (rimO * 0.8 + rimX) * vis;',
  // Filaments: fine radial hairs, long out beyond the rim, short within it.
  // Everything out beyond the rim dies away before the quad ends, so no edge shows.
  '  float win = 1.0 - smoothstep(1.8, 4.7, t);',
  '  float fo = pow(dwn(d * 20.0 + vec2(uT * 0.25, 3.0)), 2.4) * smoothstep(0.0, 0.08, t - 1.0) * exp(-max(t - 1.0, 0.0) / (0.4 + 1.3 * dwn(d * 4.0 + 7.0))) * vis * win;',
  '  float fi = pow(dwn(d * 26.0 + vec2(-uT * 0.25, 8.0)), 2.0) * smoothstep(0.0, 0.06, -1.0 - t) * exp(-max(-1.0 - t, 0.0) / 0.6) * vis;',
  '  float soft = exp(-dw2((r - uR) / (uB * 2.8))) * vis * win * smoothstep(uDie * 0.9, uDie * 1.6, r);',
  '  float glint = exp(-dw2(dwa(an, uGlint) / 0.3)) * (rimO + rimI * 0.7 + inBand * 0.25) * vis;',
  '  float back = inBand * 0.5 * vis;',
  '  float rc = clamp(rim * 0.5, 0.0, 0.6);',
  '  vec3 add = uHot * (rim * 0.7 + beadK + g * 0.4 + glint * 0.9 + head * (inBand * 0.8 + rimO + rimI) * 1.4 + uFlare * inBand * 0.15) + uGold * (fo * 1.0 + fi * 0.8 + soft * 0.06 + g * 1.1 + uFlare * (inBand * 0.3 + soft * 0.2));',
  '  float al = back + rc + fo * 0.25 + fi * 0.2 + g * 0.3 + soft * 0.2;',
  '  vec3 col = uBack * back + uAmber * (rc + fo * 0.25 + fi * 0.2 + soft * 0.2) + add;',
  '  gl_FragColor = vec4(col, clamp(al, 0.0, 0.92)) * uFade;',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'].join('\n');

// Runes drawn once into a strip of 24 cells, white on black: a few of the same strokes (a stem, branches,
// bars, a ring, a chevron) combined differently so no two match. Shared across stages.
var DAWN_RUNES = null;
function dawnGlyph(g, rnd, i) {
  var kind = i % 8, k, y0, dx, dy;
  function ln(ax, ay, bx, by) { g.beginPath(); g.moveTo(ax, ay); g.lineTo(bx, by); g.stroke(); }
  function ring(x, y, r) { g.beginPath(); g.arc(x, y, r, 0, 6.2832); g.stroke(); }
  if (kind === 0 || kind === 3 || kind === 5) {
    ln(0, -22, 0, 22);
    for (k = 0; k < 2 + (i % 2); k++) { y0 = -18 + rnd() * 26; dx = (rnd() < 0.5 ? -1 : 1) * (9 + rnd() * 7); dy = (rnd() < 0.5 ? -1 : 1) * (7 + rnd() * 9); ln(0, y0, dx, y0 + dy); }
    if (kind === 3) ln(-11, -4 + rnd() * 8, 11, -4 + rnd() * 8);
    if (kind === 5) ring(0, -15, 5);
  } else if (kind === 1) { ring(0, 0, 10); g.beginPath(); g.arc(0, 0, 2.2, 0, 6.2832); g.fill(); for (k = 0; k < 4; k++) { var a = k * 1.5708 + 0.785; ln(Math.cos(a) * 14, Math.sin(a) * 14, Math.cos(a) * 20, Math.sin(a) * 20); } }
  else if (kind === 2) { ln(-13, 14, 0, -20); ln(0, -20, 13, 14); ln(-13, 14, 13, 14); if (i > 8) ln(0, -20, 0, 14); }
  else if (kind === 4) { ln(0, -22, 12, 0); ln(12, 0, 0, 22); ln(0, 22, -12, 0); ln(-12, 0, 0, -22); g.beginPath(); g.arc(0, 0, 2.5, 0, 6.2832); g.fill(); }
  else if (kind === 6) { g.beginPath(); g.arc(0, 4, 14, 3.4, 6.0); g.stroke(); ln(-12, -8, 0, -20); ln(0, -20, 12, -8); ln(0, -4, 0, 22); }
  else { ln(-12, -18, 12, 18); ln(12, -18, -12, 18); ln(-12, 0, 12, 0); if (i > 8) ring(0, -19, 3.5); }
}
function dawnRuneTex(T) {
  if (DAWN_RUNES) return DAWN_RUNES;
  var N = 24, C = 64, c = canvas(N * C, C), g = c.getContext('2d'), rnd = mulberry(2026);
  g.fillStyle = '#000'; g.fillRect(0, 0, N * C, C);
  g.lineCap = 'round'; g.lineJoin = 'round'; g.strokeStyle = '#fff'; g.fillStyle = '#fff'; g.lineWidth = 4.5; g.shadowColor = '#fff'; g.shadowBlur = 5;
  for (var i = 0; i < N; i++) { g.save(); g.translate(i * C + C / 2, C / 2); dawnGlyph(g, rnd, i); g.restore(); }
  DAWN_RUNES = new T.CanvasTexture(c);
  DAWN_RUNES.generateMipmaps = false; DAWN_RUNES.minFilter = T.LinearFilter; DAWN_RUNES.magFilter = T.LinearFilter; share(DAWN_RUNES);
  return DAWN_RUNES;
}

// R: the ring's radius, B: half the band's width, cells: how many runes round it, order: draw order,
// die: the radius of the die under it, which the glow keeps off.
// Its own pieces must show over anything on the page, so it ignores depth.
function dawnHalo(st, R, B, cells, order, die) {
  var T = st.T, uni = { uT: { value: 0 }, uR: { value: R }, uB: { value: B }, uForm: { value: 0 }, uFade: { value: 0 }, uGlint: { value: 0 }, uRot: { value: 0 }, uFlare: { value: 0 },
    uCells: { value: cells }, uDie: { value: die }, uHot: dawnCol(T, DAWN.hot), uGold: dawnCol(T, DAWN.gold), uAmber: dawnCol(T, DAWN.amber), uBack: dawnCol(T, DAWN.back), uRune: { value: dawnRuneTex(T) } };
  var ext = R + B * 5, mesh = new T.Mesh(new T.PlaneGeometry(ext * 2, ext * 2), dawnMat(st, DAWN_HALO_VS, DAWN_HALO_FS, uni, {}, { depthTest: false }));
  mesh.rotation.x = -Math.PI / 2; mesh.renderOrder = order; mesh.frustumCulled = false; mesh.visible = false;
  return { mesh: mesh, uni: uni };
}

// ---------- The flare star ----------
// A core of light with four long spikes, lying flat over the die. It has a hole over the die's own
// footprint, so the bright part sits round the die: the camera looks straight down, and light over the
// die would bury its number. S is the die's size and every width here is in units of it.
var DAWN_STAR_VS = 'varying vec2 vP; void main(){ vP = position.xy; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }';
var DAWN_STAR_FS = [
  'uniform float uA, uRot, uW, uL, uS, uH0, uH1, uExt; uniform vec3 uHot, uGold; varying vec2 vP;',
  'void main(){',
  '  vec2 p = vP / uS; float r = length(p);',
  '  float c = cos(uRot), sn = sin(uRot); vec2 q = vec2(c * p.x - sn * p.y, sn * p.x + c * p.y);',
  '  float core = exp(-r * r / (uW * uW));',
  '  float a1 = exp(-q.y * q.y / 0.012) * exp(-abs(q.x) / uL), a2 = exp(-q.x * q.x / 0.012) * exp(-abs(q.y) / uL);',
  '  vec2 d = vec2(q.x + q.y, q.x - q.y) * 0.7071;',
  '  float a3 = 0.5 * (exp(-d.y * d.y / 0.02) * exp(-abs(d.x) / (uL * 0.6)) + exp(-d.x * d.x / 0.02) * exp(-abs(d.y) / (uL * 0.6)));',
  '  float hole = smoothstep(uH0, uH1, r);',
  '  float win = 1.0 - smoothstep(0.55, 1.0, length(vP) / uExt);',
  '  float I = (core * 0.9 + (a1 + a2) * 0.7 + a3) * hole * win * uA;',
  '  gl_FragColor = vec4(mix(uGold, uHot, 0.4) * I, 0.0);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'].join('\n');
// ext: the quad's half-width, s: the die's size, h0 and h1: where the hole ends and the light is whole.
function dawnStar(st, ext, s, h0, h1, order) {
  var T = st.T, uni = { uA: { value: 0 }, uRot: { value: 0 }, uW: { value: 1.4 }, uL: { value: 3 }, uS: { value: s }, uH0: { value: h0 }, uH1: { value: h1 }, uExt: { value: ext },
    uHot: dawnCol(T, DAWN.hot), uGold: dawnCol(T, DAWN.gold) };
  var mesh = new T.Mesh(new T.PlaneGeometry(ext * 2, ext * 2), dawnMat(st, DAWN_STAR_VS, DAWN_STAR_FS, uni, {}, { depthTest: false }));
  mesh.rotation.x = -Math.PI / 2; mesh.renderOrder = order; mesh.frustumCulled = false; mesh.visible = false;
  return { mesh: mesh, uni: uni };
}

// ---------- The shaft between dice (echo) ----------
// A thin ribbon of light lying flat between two points: a bright head runs from one end to the other,
// the body behind it holds a moment, then the tail catches up and it is gone.
var DAWN_BEAM_VS = 'varying vec2 vUv; void main(){ vUv = vec2(position.x + 0.5, position.z * 2.0); gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }';
var DAWN_BEAM_FS = [
  'uniform float uHead, uTail, uFade, uT; uniform vec3 uHot, uGold; varying vec2 vUv;',
  DAWN_NOISE,
  'void main(){',
  '  float u = vUv.x, v = vUv.y;',
  '  float body = smoothstep(uTail - 0.02, uTail + 0.14, u) * (1.0 - smoothstep(uHead - 0.06, uHead, u));',
  '  float head = exp(-dw2((u - uHead + 0.012) / 0.03)) * step(u, uHead + 0.05) * step(uTail, 0.98);',
  '  float core = exp(-v * v * 26.0), wide = exp(-v * v * 4.0);',
  '  float flow = 0.7 + 0.3 * dwn(vec2(u * 30.0 - uT * 9.0, v * 2.5));',
  '  float I = (body * (core * 1.5 * flow + wide * 0.35) + head * (core * 2.0 + wide * 0.9)) * uFade;',
  '  float cov = clamp((body * wide * 0.4 + head * wide * 0.3) * uFade, 0.0, 0.6);',
  '  gl_FragColor = vec4(uGold * cov * 0.9 + mix(uGold, uHot, clamp(core, 0.0, 1.0)) * I * 0.6, cov);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'].join('\n');
function dawnBeam(st, order) {
  var T = st.T, uni = { uHead: { value: 0 }, uTail: { value: 0 }, uFade: { value: 0 }, uT: { value: 0 }, uHot: dawnCol(T, DAWN.hot), uGold: dawnCol(T, DAWN.gold) };
  var geo = new T.PlaneGeometry(1, 1); geo.rotateX(-Math.PI / 2);
  var mesh = new T.Mesh(geo, dawnMat(st, DAWN_BEAM_VS, DAWN_BEAM_FS, uni, {}, { depthTest: false }));
  mesh.renderOrder = order; mesh.frustumCulled = false; mesh.visible = false;
  return { mesh: mesh, uni: uni,
    // From (ax, az) to (bx, bz) at height y, width w across.
    aim: function (ax, az, bx, bz, y, w) {
      var dx = bx - ax, dz = bz - az, L = Math.hypot(dx, dz) || 1;
      mesh.position.set((ax + bx) / 2, y, (az + bz) / 2); mesh.rotation.y = -Math.atan2(dz, dx); mesh.scale.set(L, 1, w);
    } };
}

// Light only adds, so on a pale stone a pale gold number washes out: there it burns a deeper amber.
// The number drawn lit in gold. The engine's own number light cores every stroke with near-white, which
// reads cream on a dark stone; this one keeps the gold and only warms the very middle of the stroke.
function dawnStroke(g, paths) { paths.forEach(function (pl) { g.beginPath(); g.moveTo(pl[0][0], pl[0][1]); for (var i = 1; i < pl.length; i++) g.lineTo(pl[i][0], pl[i][1]); g.stroke(); }); }
function dawnNumber(g, lay, col) {
  if (lay.style === 'classic') { g.shadowColor = col; g.shadowBlur = lay.h * 0.16; g.fillStyle = col; paintNumeral(g, lay, 'classic'); g.shadowBlur = 0; return; }
  var w = strokeW(lay);
  g.lineJoin = 'miter'; g.miterLimit = 3; g.lineCap = 'round';
  g.shadowColor = col; g.shadowBlur = w * 1.5; g.strokeStyle = hexA(col, 0.6); g.lineWidth = w * 1.35; dawnStroke(g, lay.strokes);
  g.strokeStyle = col; g.shadowBlur = w * 0.6; g.lineWidth = w * 1.1; dawnStroke(g, lay.strokes);
  g.shadowBlur = 0; g.strokeStyle = mix(col, '#fff2c8', 0.28); g.lineWidth = w * 0.45; dawnStroke(g, lay.strokes);
}
// The number left lit at rest, a little under the engine's own brightness so gold stays gold.
function dawnRest(die, fi, col) { var gf = glowFace(die, fi); gf.clear(); dawnNumber(gf.g, gf.lay, col); gf.commit(1.05); }
function dawnNumCol(th) {
  var g0 = th && th.base && th.base.grad ? parseInt(th.base.grad[0].slice(1), 16) : 0;
  return (0.3 * (g0 >> 16 & 255) + 0.59 * (g0 >> 8 & 255) + 0.11 * (g0 & 255)) / 255 > 0.62 ? DAWN.numDeep : DAWN.num;
}

// ---------- The effect ----------
function fxDawn(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, END = 4.7;
  o = o || {};
  var rnd = mulberry(3301 + Math.floor(Math.random() * 1e6));
  g.updateMatrixWorld(true);
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), top = worldOf(die, die.faces[fi].c), NUM = dawnNumCol(th);
  // The camera is not above the die, so a point up in the air shows over the page a little further out
  // than it stands: the shafts start where the die is seen, and the places to keep clear likewise.
  var camY = st.cam.position.y, pm = camY / Math.max(1, camY - p0.y), c = new T.Vector3(p0.x * pm, 0, p0.z * pm);
  var clr = (o.clear || []).slice(0, 12).map(function (k) { return new T.Vector3(k.x * pm, k.z * pm, k.r * pm); });
  function half(n) { return hd ? n : Math.ceil(n / 2); }

  // The halo forms from TH0 and is whole at BEAT, the big moment: light flares and a pulse runs out.
  var TH0 = 1.7, BEAT = 2.45, YH = top.y + s * 0.78;
  var sky = dawnSky(st, c, s, hd, clr), U = sky.uni;
  var motes = dawnMotes(st, hd ? 120 : 60, s, sky, rnd, clr, p0.x, p0.z);
  var halo = dawnHalo(st, s * 2.15, s * 0.28, 24, 8, s), hu = halo.uni;
  halo.mesh.position.set(p0.x, YH, p0.z);
  // A charged rim of light round the die (centred inside, so the die hides its middle) and a lens
  // star for the flare, with a hole over the die.
  var rim = glow(st, spriteTex(T), DAWN.gold, s * 4.2), star = dawnStar(st, s * 7.5, s, 1.0, 1.9, 10);
  rim.position.copy(p0); star.mesh.position.set(p0.x, YH, p0.z);
  // One light, low and to the lower right and rising like a sun, so the die's sides catch it and the
  // top face, which the camera looks straight down at, gets no glare.
  var light = new T.PointLight(0xffc870, 0, s * 18, 1.3); st.scene.add(light);
  st.scene.add(sky.mesh, motes.mesh, halo.mesh, rim, star.mesh);

  var fcT = null, sparked = false;
  function lightNumber() { var gf = glowFace(die, fi); gf.clear(); dawnNumber(gf.g, gf.lay, NUM); gf.commit(0); fcT = glowLayer(die, fi); }
  var bump = Math.PI * 2;

  return { update: function (dt) {
    t += dt;
    if (t >= END) {
      drop(st, [sky.mesh, motes.mesh, halo.mesh, rim, star.mesh, light]);
      g.position.copy(p0); g.quaternion.copy(q0); g.updateMatrixWorld(true);
      shadeDie(die, 1); tintDie(die, th.burn, 0); dawnRest(die, fi, NUM);
      return false;
    }
    var gk = st.cine ? 1 : 1.6;
    var wash = smooth(0.0, 0.95, t) * (1 - smooth(3.3, 4.5, t));
    var lit = smooth(1.0, 2.1, t) * (1 - smooth(3.8, 4.6, t));
    var fl = env4(t, 2.3, BEAT, BEAT + 0.08, 3.0);
    var rk = clamp01((t - BEAT) / 1.1), away = 1 - smooth(3.7, 4.5, t);

    // The sky: dark cloud, the opening tearing wider, shafts shooting out and holding, a pulse and a ring
    // running out at the beat, and the wash lifting at the end.
    U.uT.value = t; U.uSpin.value = 0.1 + t * 0.07;
    U.uWash.value = wash;
    U.uOpen.value = s * (2.3 * out3((t - 0.7) / 1.1) + 1.2 * smooth(1.9, 3.2, t) + 8 * smooth(3.4, 4.5, t));
    U.uRay.value = smooth(1.0, 1.9, t) * (1 - smooth(3.5, 4.5, t)) * (0.9 + 0.1 * Math.sin(t * 4.1)) * (1 + 0.3 * fl);
    U.uReach.value = s * (3 + 9.5 * out3((t - 1.0) / 1.3));
    U.uPool.value = 0.9 * smooth(0.9, 1.8, t) * (1 - smooth(3.6, 4.5, t)) + 0.45 * fl;
    U.uSurgeR.value = s * (1.5 + 13 * out3(rk)); U.uSurgeA.value = t >= BEAT ? 1.8 * Math.pow(1 - rk, 1.5) : 0;
    U.uRingR.value = s * (1.6 + 10 * out3(rk)); U.uRingA.value = t >= BEAT ? (1 - rk) * (1 - rk) : 0;
    sky.mesh.visible = U.uWash.value > 0.003 || U.uRay.value > 0.003 || U.uPool.value > 0.003;

    // Dust: drawn in toward the halo as it forms, thrown out at the beat.
    motes.step(dt, t, p0.x, p0.z, smooth(0.8, 1.6, t) * away, 1.1 * env4(t, 1.8, 2.2, BEAT, 2.7), s * 2.5, 7 * env4(t, BEAT, BEAT + 0.05, 2.6, 3.1), 0.5 * env4(t, 1.7, 2.2, BEAT, 3.0));

    // The halo draws itself, descends from above (so it starts a little larger) and hovers.
    var hk = clamp01((t - TH0) / 0.9);
    hu.uT.value = t; hu.uForm.value = smooth(TH0, BEAT, t); hu.uFade.value = t >= TH0 ? away : 0; hu.uFlare.value = fl * 0.6;
    hu.uGlint.value = t * 1.7; hu.uRot.value = t * 0.015;
    halo.mesh.visible = hu.uForm.value > 0.001 && hu.uFade.value > 0.003;
    halo.mesh.scale.setScalar(1 + 0.3 * (1 - out3(hk)) + 0.05 * fl);
    halo.mesh.position.y = YH + s * (0.06 * Math.sin(t * 2.3) + 0.5 * smooth(3.7, 4.5, t) + 0.2 * (1 - hk));

    rim.material.opacity = (0.1 * lit + 0.12 * fl) * gk;
    var sk = t >= BEAT ? Math.exp(-(t - BEAT) * 5) : 0, su = star.uni;
    su.uA.value = 0.7 * sk; su.uRot.value = (t - BEAT) * 0.5; su.uW.value = 1.5 + 1.4 * (1 - sk); su.uL.value = 3 + 2.5 * (1 - sk);
    rim.visible = rim.material.opacity > 0.003; star.mesh.visible = su.uA.value > 0.003;

    if (!sparked && t >= BEAT) {
      sparked = true;
      st.fx.push(spray(st, { n: half(80), colors: [DAWN.hot, DAWN.gold, '#ffffff'], size: s * 0.07, life: [0.6, 1.3], drag: 1.1, g: -0.8, noFloor: true,
        spawn: function (i, r) { var a = r() * bump, rr = s * 2.15 * (0.9 + r() * 0.2), v = 1.5 + r() * 3.2; return { p: [p0.x + Math.cos(a) * rr, YH, p0.z + Math.sin(a) * rr], v: [Math.cos(a) * v, 0.5 + r() * 2.2, Math.sin(a) * v] }; } }));
    }

    // The die: shadowed under the storm, then lit gold by the rising light, which hovers it a little
    // while the halo is over it. It comes back down to where it landed.
    var lift = (s * 0.11 + s * 0.012 * Math.sin(t * 2.6)) * smooth(2.2, 2.8, t) * (1 - smooth(3.7, 4.4, t));
    g.position.set(p0.x, p0.y + lift, p0.z);
    shadeDie(die, 1 - 0.3 * wash * (1 - lit));
    tintDie(die, DAWN.gold, 0.2 * lit + 0.1 * fl);
    var el = 0.18 + 0.44 * smooth(1.0, 3.0, t), D = s * 6.5;
    light.position.set(p0.x + Math.cos(el) * 0.62 * D, p0.y + Math.sin(el) * D + s * 0.5, p0.z + Math.cos(el) * 0.78 * D);
    light.intensity = (70 * smooth(1.0, 2.0, t) + 20 * fl) * away * (0.94 + 0.06 * Math.sin(t * 9));
    if (!fcT && t >= 0.95) lightNumber();
    if (fcT) fcT.mat.emissiveIntensity = (1.05 + 0.15 * (1 - smooth(3.4, 4.4, t))) * smooth(0.95, 2.0, t) + 0.2 * fl;
    return true;
  } };
}

// The echo: a thin shaft reaches over from the grand die (if there is one) and a small halo lights over
// this die. Alone, the halo just forms.
function fxDawnEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), top = worldOf(die, die.faces[fi].c), NUM = dawnNumCol(th), at = new T.Vector3(p0.x, 0, p0.z);
  var from = e && e.from ? e.from : null, YH = top.y + s * 0.7, arrive = 0, beam = null;
  if (from) {
    var sf = e.fromFi >= 0 ? e.fromFi : 0, sp = worldOf(from, from.faces[sf].c), L = Math.hypot(sp.x - p0.x, sp.z - p0.z);
    arrive = Math.min(0.42, Math.max(0.2, L / 36));
    beam = dawnBeam(st, 60); beam.aim(sp.x, sp.z, p0.x, p0.z, (sp.y + from.size * 0.78 + YH) / 2, s * 0.5); st.scene.add(beam.mesh);
  }
  var h0 = from ? arrive - 0.06 : 0.05, END = h0 + 1.5, BEAT = h0 + 0.5;
  var halo = dawnHalo(st, s * 1.5, s * 0.19, 15, 62, s), hu = halo.uni;
  halo.mesh.position.set(p0.x, YH, p0.z);
  var dim = dimmer(st, at, s * 3.6);
  var rim = glow(st, spriteTex(T), DAWN.gold, s * 3.2), star = dawnStar(st, s * 4.5, s, 0.9, 1.5, 63);
  rim.renderOrder = 61; star.mesh.position.set(p0.x, YH, p0.z); rim.position.copy(p0);
  st.scene.add(halo.mesh, rim, star.mesh);
  var fcT = null, sparked = false;
  return { update: function (dt) {
    t += dt;
    if (t >= END) {
      drop(st, [halo.mesh, dim, rim, star.mesh, beam && beam.mesh]);
      g.position.copy(p0); g.quaternion.copy(q0);
      tintDie(die, th.burn, 0); dawnRest(die, fi, NUM);
      return false;
    }
    var gk = st.cine ? 1 : 1.6, away = 1 - smooth(END - 0.55, END - 0.08, t), fl = env4(t, BEAT - 0.1, BEAT, BEAT + 0.05, BEAT + 0.5);
    if (beam) {
      var bu = beam.uni;
      bu.uT.value = t; bu.uHead.value = out3(t / arrive); bu.uTail.value = smooth(arrive + 0.12, arrive + 0.55, t) * 1.02; bu.uFade.value = 1 - smooth(arrive + 0.4, arrive + 0.6, t);
      beam.mesh.visible = bu.uFade.value > 0.003 && bu.uTail.value < 1;
    }
    hu.uT.value = t; hu.uForm.value = smooth(h0, BEAT, t); hu.uFade.value = t >= h0 ? away : 0; hu.uFlare.value = fl * 0.7;
    hu.uGlint.value = t * 2.2; hu.uRot.value = t * 0.02;
    halo.mesh.visible = hu.uForm.value > 0.001 && hu.uFade.value > 0.003;
    var hk = clamp01((t - h0) / 0.6);
    halo.mesh.scale.setScalar(1 + 0.2 * (1 - out3(hk)) + 0.04 * fl);
    halo.mesh.position.y = YH + s * (0.04 * Math.sin(t * 2.6) + 0.3 * smooth(END - 0.55, END, t));
    dim.material.opacity = 0.42 * env4(t, h0, h0 + 0.4, END - 0.6, END - 0.05);
    rim.material.opacity = (0.08 * smooth(h0, h0 + 0.5, t) * away + 0.12 * fl) * gk; rim.visible = rim.material.opacity > 0.003;
    var sk = t >= BEAT ? Math.exp(-(t - BEAT) * 6) : 0, ak = from && t >= arrive ? Math.exp(-(t - arrive) * 9) : 0;
    var eu = star.uni; eu.uA.value = 0.6 * Math.max(sk, ak * 0.7); eu.uRot.value = t * 0.5; eu.uW.value = 1.0 + 0.8 * (1 - sk); eu.uL.value = 2 + 1.5 * (1 - sk); star.mesh.visible = eu.uA.value > 0.003;
    if (!sparked && t >= BEAT) {
      sparked = true;
      st.fx.push(spray(st, { n: hd ? 26 : 13, colors: [DAWN.hot, DAWN.gold], size: s * 0.06, life: [0.4, 0.9], drag: 1.2, g: -0.6, noFloor: true,
        spawn: function (i, r) { var a = r() * 6.2832, rr = s * 1.5, v = 1 + r() * 2; return { p: [p0.x + Math.cos(a) * rr, YH, p0.z + Math.sin(a) * rr], v: [Math.cos(a) * v, 0.4 + r() * 1.5, Math.sin(a) * v] }; } }));
    }
    tintDie(die, DAWN.gold, 0.3 * env4(t, h0 + 0.1, BEAT, END - 0.7, END - 0.05) + 0.1 * fl);
    if (!fcT && t >= h0 + 0.1) { var gf = glowFace(die, fi); gf.clear(); dawnNumber(gf.g, gf.lay, NUM); gf.commit(0); fcT = glowLayer(die, fi); }
    if (fcT) fcT.mat.emissiveIntensity = (1.05 + 0.15 * (1 - smooth(END - 0.8, END - 0.1, t))) * smooth(h0 + 0.1, h0 + 0.8, t) + 0.2 * fl;
    return true;
  } };
}
registerCrit('dawn', { name: 'Dawnbreak', kind: 'succ',
  what: 'The page dims, shafts of gold light break through, and a halo forms over the die.',
  fx: fxDawn, beat: 2.45, peak: 3.1,
  echo: fxDawnEcho, echoWhat: 'A small halo lights over the die.' });
