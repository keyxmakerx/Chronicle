// ---------- Crit success: Phoenix ----------
// The 20 catches fire and flame licks out of the die's seams. A phoenix of fire erupts from it: a
// white-hot breast and head over the die, two great wings of layered flame feathers unfurling across the
// page, and a long tail of streaming plumes. It beats its wings once, throwing embers and a ring of
// heat-light over the page and scorching faint feather marks into it, then folds its wings and spirals
// down into the die in a vortex of flame. A flash, and the die cools like forge-hot metal with the 20
// left burning gold.
//
// Every flame is one instance of a bent strip drawn with a fire shader (turbulence streaming from root
// to tip, a temperature ramp from white through gold, orange and red to smoke), so the whole bird costs
// two draw calls. Flames blend premultiplied: a hot core adds light like any glow, but the body of the
// flame and its smoke also cover what is under them, which is what keeps orange fire orange on a white page.
import {
  canvas, clamp01, cutPaths, dimmer, drop, env4, faceToLocal, glow, glowFace, glowNumeral,
  glowStrokes, inout, mix, mulberry, numberOnto, out3, registerCrit, shadeDie, smooth, spriteTex,
  starTex, strokeW, tintDie, worldOf, share
} from '../engine.js';

var PHX = { white: '#fff3c4', gold: '#ffc34a', orange: '#ff7a1a', red: '#c4200f', smoke: '#2e2420', char: '#1f0f08' };

var PHX_GLSL = [
  'uniform float uTime; uniform vec3 uWhite, uGold, uOrange, uRed, uSmoke;',
  'float phh(vec2 p){ return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }',
  'float phn(vec2 p){ vec2 i = floor(p), f = fract(p); vec2 u = f * f * (3.0 - 2.0 * f); return mix(mix(phh(i), phh(i + vec2(1.0, 0.0)), u.x), mix(phh(i + vec2(0.0, 1.0)), phh(i + vec2(1.0, 1.0)), u.x), u.y); }',
  'float phf(vec2 p){ float s = 0.0, a = 0.5, w = 0.0; for (int i = 0; i < OCT; i++) { s += a * phn(p); w += a; p = p * 2.07 + vec2(1.7, 9.2); a *= 0.5; } return s / w; }',
  // Temperature to colour, graded the way a flame is: dull red at the cool edge, through orange and gold, to white at the core.
  'vec3 phRamp(float t){',
  '  vec3 c = uRed * 0.5 * smoothstep(0.04, 0.22, t);',
  '  c = mix(c, uRed * 1.1, smoothstep(0.18, 0.34, t));',
  '  c = mix(c, uOrange * 1.45, smoothstep(0.32, 0.52, t));',
  '  c = mix(c, uGold * 1.6, smoothstep(0.52, 0.74, t));',
  '  return mix(c, uWhite * 2.6, smoothstep(0.76, 0.98, t)); }'
].join('\n');

// Dice to keep in view (o.clear): a soft hole round each, in every layer that could cover one. A point
// is measured where it appears on the page, since a flame in the air above a die is seen over that die.
var PHX_CLEAR = [
  'uniform vec3 uClr[8]; uniform float uCam;',
  'float phClear(vec3 w){ vec2 g = w.xz * (uCam / max(uCam - w.y, 1.0)); float m = 1.0;',
  '  for (int i = 0; i < 8; i++) { float r = uClr[i].z; if (r > 0.0) m *= smoothstep(r * 0.8, r * 1.6, length(g - uClr[i].xy)); }',
  '  return m; }'
].join('\n');

// Half-width of a flame along its length (u: root 0, tip 1), by kind: 0 a feather (narrow quill, broad
// vane, pointed tip), 1 a tongue of flame (broad base, licking point), 2 a streamer (thin shaft flaring
// into a flame at its end, for the tail and crest).
var PHX_SHAPE = [
  'float phProf(float u, float kind){',
  '  float r = max(1.0 - u, 0.0);',
  '  if (kind < 0.5) return (0.3 + 0.7 * smoothstep(0.0, 0.3, u)) * pow(r, 0.55);',
  '  if (kind < 1.5) return (0.7 + 0.3 * smoothstep(0.0, 0.2, u)) * pow(r, 0.85);',
  '  return (0.2 + 0.8 * smoothstep(0.3, 0.78, u)) * pow(r, 0.4); }'
].join('\n');

var PHX_SHEET_VS = [
  'uniform float uTime; uniform float uWide;',
  'attribute vec4 aShape; attribute vec4 aFire; attribute vec4 aMode;',
  'varying vec2 vUv; varying vec4 vFire; varying vec4 vMode; varying float vLen; varying vec3 vW;',
  // The spine: a steady bend plus a wave that travels from root to tip, so a plume ripples like fire in a
  // draught. Integrated in steps, because the heading changes along the length.
  'float phAng(float u){ return aShape.z * u + aMode.y * sin(u * 5.5 - uTime * 6.5 + aFire.x * 6.2832) * u; }',
  'void main(){',
  '  float u = position.x, v = position.z, du = u / 10.0; vec2 p = vec2(0.0);',
  '  for (int i = 0; i < 10; i++) { float a = phAng((float(i) + 0.5) * du); p += vec2(cos(a), sin(a)) * du; }',
  '  float a = phAng(u); vec2 n = vec2(-sin(a), cos(a));',
  '  vec3 lp = vec3(p.x, aShape.w * u * u, p.y) * aShape.x + vec3(n.x, 0.0, n.y) * v * aShape.y * uWide;',
  '  gl_Position = projectionMatrix * modelViewMatrix * instanceMatrix * vec4(lp, 1.0);',
  '  vW = (instanceMatrix * vec4(lp, 1.0)).xyz;',
  '  vUv = vec2(u, v * 2.0); vFire = aFire; vMode = aMode; vLen = aShape.x / max(aShape.y, 0.001);',
  '}'
].join('\n');

// aFire: seed, heat, alpha, grow (how much of the length has caught). aMode: kind, wave, smoke, flow speed.
var PHX_FLAME_FS = [
  PHX_GLSL, PHX_SHAPE, PHX_CLEAR,
  'uniform float uGain, uOpa;',
  'varying vec2 vUv; varying vec4 vFire; varying vec4 vMode; varying float vLen; varying vec3 vW;',
  'void main(){',
  '  float u = vUv.x, v = vUv.y, seed = vFire.x, heat = vFire.y, al = vFire.z, grow = vFire.w, kind = vMode.x;',
  '  float cm = phClear(vW); al *= cm;',
  '  if (al < 0.004) discard;',
  '  float tt = uTime * vMode.w;',
  // Turbulence streaming from root to tip, and a finer layer warped by it: the curl of real flame.
  '  vec2 q = vec2(u * vLen * 1.25 - tt * 2.6, v * 0.85 + seed * 9.0);',
  '  float n1 = phf(q);',
  '  float n2 = phf(q * vec2(2.3, 1.7) + vec2(-tt * 1.9, n1 * 2.2));',
  '  float e = abs(v + (n1 - 0.5) * (0.25 + 0.95 * u)) / max(phProf(u, kind), 0.001);',
  '  float d = smoothstep(1.0, 0.5, e + (n2 - 0.5) * (0.5 + 0.8 * u));',
  '  float tongue = step(0.5, kind) * step(kind, 1.5);',
  // Barbs angled back from the shaft: what makes a flame read as a feather.
  '  float barb = 0.5 + 0.5 * sin((u * vLen * 2.4 - abs(v) * 1.3 + n2 * 0.8) * 18.85);',
  '  d *= 1.0 - 0.5 * (1.0 - tongue) * smoothstep(0.12, 0.55, e) * (1.0 - barb);',
  // Only the lit part of the length shows, so a flame runs out from its root as it catches.
  '  float front = grow * 1.15 - 0.08;',
  '  d *= smoothstep(front + 0.05, front - 0.1, u + (n1 - 0.5) * 0.2);',
  // Tips tear into wisps.
  '  d *= smoothstep(1.0, 0.76, u + (n2 - 0.5) * 0.5) * smoothstep(0.0, 0.07 + 0.08 * n1, u);',
  // Each flame burns at its own heat. Smooth noise, not the hash: hashing an interpolated varying turns its rounding error into pixel noise.
  '  float flick = (0.86 + 0.28 * phn(vec2(uTime * 9.0, seed * 37.0))) * (0.84 + 0.3 * phn(vec2(seed * 3.7, 1.7)));',
  '  float shaft = exp(-v * v * 45.0) * (1.0 - u) * 0.3 * (1.0 - tongue);',
  '  float temp = heat * flick * ((1.0 - e * 0.6) * (1.0 - u * 0.7) * (0.45 + 0.8 * n2) + shaft);',
  '  float fire = d * smoothstep(0.1, 0.3, temp);',
  '  float smoke = d * (1.0 - smoothstep(0.04, 0.26, temp)) * vMode.z;',
  '  vec3 col = phRamp(temp) * fire * uGain + uSmoke * smoke * 0.5;',
  '  gl_FragColor = vec4(col, clamp(fire * uOpa + smoke * 0.5, 0.0, 1.0)) * al;',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// The same flames drawn wide and soft and purely added: the glow round fire, which Basic has no bloom to give.
var PHX_GLOW_FS = [
  PHX_GLSL, PHX_SHAPE, PHX_CLEAR,
  'uniform float uGain, uWide;',
  'varying vec2 vUv; varying vec4 vFire; varying vec4 vMode; varying float vLen; varying vec3 vW;',
  'void main(){',
  '  float u = vUv.x, al = vFire.z * phClear(vW); if (al < 0.004) discard;',
  '  float e = abs(vUv.y * uWide) / max(phProf(u, vMode.x), 0.08);',
  '  float g = exp(-e * e * 0.7) * pow(max(1.0 - u, 0.0), 1.2) * smoothstep(0.0, 0.15, u) * smoothstep(vFire.w * 1.15 - 0.02, vFire.w * 1.15 - 0.25, u);',
  '  float fl = 0.85 + 0.3 * phn(vec2(uTime * 7.0, vFire.x * 23.0));',
  '  gl_FragColor = vec4(phRamp(vFire.y * 0.62) * g * fl * uGain * al, 0.0);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// A feather branded into the page by the downstroke: charred barbs, edges glowing as they cool.
var PHX_SCORCH_FS = [
  PHX_GLSL, PHX_SHAPE, PHX_CLEAR,
  'uniform float uFade, uEmber; uniform vec3 uChar;',
  'varying vec2 vUv; varying vec4 vFire; varying vec4 vMode; varying float vLen; varying vec3 vW;',
  'void main(){',
  '  float u = vUv.x, v = vUv.y, seed = vFire.x, cm = phClear(vW); if (vFire.z * cm < 0.004 || uFade < 0.004) discard;',
  '  vec2 q = vec2(u * vLen * 1.25, v * 0.85 + seed * 9.0);',
  '  float n1 = phf(q * 1.3), n2 = phf(q * vec2(3.1, 2.2) + n1);',
  '  float e = abs(v + (n1 - 0.5) * 0.4 * u) / max(phProf(u, 0.0), 0.001);',
  '  float d = smoothstep(1.0, 0.6, e + (n2 - 0.5) * 0.5) * smoothstep(1.0, 0.7, u + (n2 - 0.5) * 0.4) * smoothstep(0.0, 0.12, u);',
  '  float barb = 0.5 + 0.5 * sin((u * vLen * 2.4 - abs(v) * 1.3 + n2 * 0.8) * 18.85);',
  '  float burn = d * (0.4 + 0.6 * barb) * (0.6 + 0.4 * n2) * uFade * 0.17;',
  '  float rim = smoothstep(0.2, 0.0, abs(e - 0.72 + (n1 - 0.5) * 0.3)) * d;',
  '  float hot = uEmber * (rim * 0.8 + d * 0.3 * smoothstep(0.55, 0.85, n2));',
  '  gl_FragColor = vec4(uChar * burn + phRamp(0.22 + 0.35 * hot) * hot, burn) * vFire.z * cm;',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// The page under the bird: darkened in the wings' shape so the fire reads on white, lit by a flickering
// pool of firelight, and crossed by rings of heat-light.
var PHX_FLOOR_VS = 'varying vec2 vP; varying vec3 vW; void main(){ vP = position.xy; vW = (modelMatrix * vec4(position, 1.0)).xyz; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }';
var PHX_FLOOR_FS = [
  PHX_GLSL, PHX_CLEAR,
  'uniform float uS, uShade, uPool; uniform vec4 uRing; uniform vec2 uSpan; varying vec2 vP; varying vec3 vW;',
  'void main(){',
  '  vec2 w = vec2(vP.x, -vP.y) / uS; float r = length(w), el = length(w / uSpan);',
  '  float shade = uShade * exp(-el * el * 1.5);',
  '  float fl = 0.7 + 0.6 * phf(w * 0.3 + vec2(uTime * 1.1, -uTime * 0.8));',
  '  float pool = uPool * fl * exp(-el * el * 2.4);',
  '  float w1 = 0.3 + uRing.x * 0.05, w2 = 0.25 + uRing.z * 0.05;',
  '  float r1 = uRing.y * exp(-pow((r - uRing.x) / w1, 2.0)) * (0.65 + 0.7 * phf(vec2(atan(w.y, w.x) * 2.5, uTime * 1.5)));',
  '  float r2 = uRing.w * exp(-pow((r - uRing.z) / w2, 2.0));',
  '  float front = uRing.y * exp(-pow((r - uRing.x - w1 * 1.6) / w1, 2.0)) + uRing.w * exp(-pow((r - uRing.z - w2 * 1.6) / w2, 2.0));',
  '  vec3 col = uOrange * pool + mix(uOrange, uGold, 0.3) * (r1 + r2);',
  '  float cm = phClear(vW);',
  '  gl_FragColor = vec4(col * cm, clamp(shade + front * 0.22, 0.0, 0.8) * cm);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// Embers and smoke: quads lying flat to the camera, which looks straight down, rewritten each frame.
var PHX_MOTE_VS = 'attribute vec2 aUv; attribute vec4 aDat; varying vec2 vUv; varying vec4 vDat; varying vec3 vW; void main(){ vUv = aUv; vDat = aDat; vW = position; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0); }';
var PHX_EMBER_FS = [
  PHX_GLSL,
  'uniform float uGain; varying vec2 vUv; varying vec4 vDat;',
  'void main(){',
  '  if (vDat.y < 0.004) discard;',
  '  float x = vUv.x, y = vUv.y * 2.0 - 1.0;',
  '  float body = exp(-y * y * 5.0) * smoothstep(0.0, 0.35, x) * smoothstep(1.0, 0.8, x);',
  '  float tmp = vDat.x * (0.7 + 0.55 * phn(vec2(uTime * 13.0, vDat.z * 61.0))) * (0.55 + 0.45 * x);',
  '  gl_FragColor = vec4(phRamp(tmp) * body * uGain, body * 0.75 * smoothstep(0.12, 0.4, tmp)) * vDat.y;',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');
var PHX_SMOKE_FS = [
  PHX_GLSL, PHX_CLEAR,
  'uniform sampler2D uMap; varying vec2 vUv; varying vec4 vDat; varying vec3 vW;',
  'void main(){',
  '  if (vDat.y < 0.004) discard;',
  '  float a = texture2D(uMap, vUv).r * vDat.y * phClear(vW);',
  // Smoke just off a flame is lit from under by it.
  '  gl_FragColor = vec4((uSmoke + uOrange * vDat.x * 0.45) * a, a);',
  '  #include <tonemapping_fragment>',
  '  #include <colorspace_fragment>',
  '}'
].join('\n');

// A soft, ragged puff for smoke, as value noise faded to its edge. Made once and kept.
var PHX_SMOKE_TEX = null;
function phoenixSmokeTex(T) {
  if (PHX_SMOKE_TEX) return PHX_SMOKE_TEX;
  var N = 64, c = canvas(N), g = c.getContext('2d'), img = g.createImageData(N, N), rnd = mulberry(29);
  function lattice(G) { var a = []; for (var i = 0; i < (G + 1) * (G + 1); i++) a.push(rnd()); return a; }
  function vn(L, G, x, y) {
    var gx = x * G, gy = y * G, ix = Math.min(G - 1, Math.floor(gx)), iy = Math.min(G - 1, Math.floor(gy)), fx = gx - ix, fy = gy - iy;
    fx = fx * fx * (3 - 2 * fx); fy = fy * fy * (3 - 2 * fy);
    var a = L[iy * (G + 1) + ix], b = L[iy * (G + 1) + ix + 1], d = L[(iy + 1) * (G + 1) + ix], e = L[(iy + 1) * (G + 1) + ix + 1];
    return a + (b - a) * fx + (d - a) * fy + (a - b - d + e) * fx * fy;
  }
  var L1 = lattice(4), L2 = lattice(8), L3 = lattice(16);
  for (var y = 0; y < N; y++) for (var x = 0; x < N; x++) {
    var u = (x + 0.5) / N, v = (y + 0.5) / N, r = Math.hypot(u - 0.5, v - 0.5) * 2;
    var n = 0.55 * vn(L1, 4, u, v) + 0.3 * vn(L2, 8, u, v) + 0.15 * vn(L3, 16, u, v);
    var k = clamp01((n - 0.3) * 2) * (1 - smooth(0.25, 1, r)), o = (y * N + x) * 4;
    img.data[o] = img.data[o + 1] = img.data[o + 2] = Math.round(k * 255); img.data[o + 3] = 255;
  }
  g.putImageData(img, 0, 0);
  PHX_SMOKE_TEX = new T.CanvasTexture(c); share(PHX_SMOKE_TEX);
  return PHX_SMOKE_TEX;
}

// Adds the places to keep clear (eight vec3: ground x, z and radius, 0 for none) to a material's extras.
function phoenixWith(extra, clr) { if (clr) extra.uClr = clr; return extra; }
// The nearest eight of the dice to keep in view, as the clear spots the shaders read: each die's
// centre as it appears on the page, and its radius.
function phoenixSpots(st, clear, at, yd) {
  var T = st.T, cy = st.cam.position.y, f = cy / (cy - yd), out = [], k;
  var near = (clear || []).slice().sort(function (a, b) { return Math.hypot(a.x - at.x, a.z - at.z) - Math.hypot(b.x - at.x, b.z - at.z); });
  for (k = 0; k < 8; k++) out.push(near[k] ? new T.Vector3(near[k].x * f, near[k].z * f, near[k].r) : new T.Vector3());
  return out;
}
// The page's darkening patch with a hole round each die to keep in view; with none it is the engine's own.
function phoenixDimmer(st, at, radius, clr, any) {
  var m = dimmer(st, at, radius);
  if (!any) return m;
  m.material.onBeforeCompile = function (sh) {
    sh.uniforms.uClr = { value: clr };
    sh.vertexShader = sh.vertexShader.replace('#include <common>', '#include <common>\nvarying vec2 vHw;')
      .replace('#include <begin_vertex>', '#include <begin_vertex>\nvHw = (modelMatrix * vec4(transformed, 1.0)).xz;');
    sh.fragmentShader = sh.fragmentShader.replace('#include <common>', '#include <common>\nuniform vec3 uClr[8]; varying vec2 vHw;')
      .replace('#include <color_fragment>', '#include <color_fragment>\nfor (int i = 0; i < 8; i++) { float cr = uClr[i].z; if (cr > 0.0) diffuseColor.a *= smoothstep(cr * 0.75, cr * 1.6, length(vHw - uClr[i].xy)); }');
  };
  return m;
}

function phoenixMat(st, hd, vs, fs, extra) {
  var T = st.T, u = { uTime: { value: 0 }, uWhite: { value: new T.Color(PHX.white) }, uGold: { value: new T.Color(PHX.gold) },
    uOrange: { value: new T.Color(PHX.orange) }, uRed: { value: new T.Color(PHX.red) }, uSmoke: { value: new T.Color(PHX.smoke) } };
  u.uClr = { value: [0, 1, 2, 3, 4, 5, 6, 7].map(function () { return new T.Vector3(); }) }; u.uCam = { value: st.cam.position.y };
  for (var k in extra) u[k] = { value: extra[k] };
  // Premultiplied: colour adds, alpha covers. One blend serves light (alpha 0), smoke (dark, alpha) and flame (both).
  return new T.ShaderMaterial({ vertexShader: vs, fragmentShader: fs, uniforms: u, defines: { OCT: hd ? 5 : 3 }, transparent: true, depthWrite: false, side: T.DoubleSide,
    blending: T.CustomBlending, blendSrc: T.OneFactor, blendDst: T.OneMinusSrcAlphaFactor, blendSrcAlpha: T.OneFactor, blendDstAlpha: T.OneMinusSrcAlphaFactor });
}

// A strip along +x from 0 to 1, width 1 across z; the vertex shader bends it into a flame.
function phoenixStrip(T, seg) {
  var pos = [], idx = [];
  for (var k = 0; k <= seg; k++) pos.push(k / seg, 0, -0.5, k / seg, 0, 0.5);
  for (k = 0; k < seg; k++) { var a = k * 2; idx.push(a, a + 1, a + 2, a + 1, a + 3, a + 2); }
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setIndex(idx);
  return geo;
}
function phoenixAttrs(T, geo, n) {
  var A = {};
  ['aShape', 'aFire', 'aMode'].forEach(function (k) { A[k] = new T.InstancedBufferAttribute(new Float32Array(n * 4), 4); A[k].setUsage(T.DynamicDrawUsage); geo.setAttribute(k, A[k]); });
  return A;
}
// Writes one flame: its root, heading (unit vector), overall scale and look (f). The strip lies as flat to
// the camera as its heading allows, since the camera looks straight down.
var PHX_UP = null;
function phoenixPut(T, M, A, i, root, dir, sc, f, X, Y, Z) {
  if (!PHX_UP) PHX_UP = new T.Vector3(0, 1, 0);
  X.copy(dir); Z.crossVectors(X, PHX_UP); if (Z.lengthSq() < 1e-6) Z.set(0, 0, 1); Z.normalize(); Y.crossVectors(Z, X);
  var m = i * 16;
  M[m] = X.x * sc; M[m + 1] = X.y * sc; M[m + 2] = X.z * sc; M[m + 3] = 0;
  M[m + 4] = Y.x * sc; M[m + 5] = Y.y * sc; M[m + 6] = Y.z * sc; M[m + 7] = 0;
  M[m + 8] = Z.x * sc; M[m + 9] = Z.y * sc; M[m + 10] = Z.z * sc; M[m + 11] = 0;
  M[m + 12] = root.x; M[m + 13] = root.y; M[m + 14] = root.z; M[m + 15] = 1;
  var j = i * 4, S = A.aShape.array, F = A.aFire.array, O = A.aMode.array;
  S[j] = f.len; S[j + 1] = f.wid; S[j + 2] = f.bend; S[j + 3] = f.lift;
  F[j] = f.seed; F[j + 1] = f.heat; F[j + 2] = f.alpha; F[j + 3] = f.grow;
  O[j] = f.kind; O[j + 1] = f.wave; O[j + 2] = f.smoke; O[j + 3] = f.flow;
}

// All the bird's flames, drawn twice from one set of instances: a wide soft glow, then the flame.
function phoenixSheets(st, n, hd, clr) {
  var T = st.T, geo = phoenixStrip(T, hd ? 18 : 11), A = phoenixAttrs(T, geo, n);
  var flameMat = phoenixMat(st, hd, PHX_SHEET_VS, PHX_FLAME_FS, phoenixWith({ uWide: 1, uGain: 1, uOpa: 0.82 }, clr));
  var glowMat = phoenixMat(st, hd, PHX_SHEET_VS, PHX_GLOW_FS, phoenixWith({ uWide: 2.4, uGain: hd ? 0.13 : 0.32 }, clr));
  var flame = new T.InstancedMesh(geo, flameMat, n), glo = new T.InstancedMesh(geo, glowMat, n);
  flame.instanceMatrix.setUsage(T.DynamicDrawUsage); glo.instanceMatrix = flame.instanceMatrix;
  flame.frustumCulled = glo.frustumCulled = false; glo.renderOrder = 4; flame.renderOrder = 5;
  var M = flame.instanceMatrix.array, F = A.aFire.array, X = new T.Vector3(), Y = new T.Vector3(), Z = new T.Vector3();
  for (var i = 0; i < M.length; i++) M[i] = 0;
  return { flame: flame, glow: glo,
    set: function (i, root, dir, sc, f) { phoenixPut(T, M, A, i, root, dir, sc, f, X, Y, Z); },
    // A hidden flame collapses to a point, so it costs no fragments.
    hide: function (i) { for (var k = i * 16; k < i * 16 + 16; k++) M[k] = 0; F[i * 4 + 2] = 0; },
    commit: function (time) {
      flame.instanceMatrix.needsUpdate = true; A.aShape.needsUpdate = A.aFire.needsUpdate = A.aMode.needsUpdate = true;
      flameMat.uniforms.uTime.value = glowMat.uniforms.uTime.value = time;
    } };
}

// Feather marks left on the page: the wing feathers where the downstroke left them, laid flat. Made
// empty at the start, so its shader is compiled then and not at the wingbeat.
function phoenixScorch(st, n, hd, clr) {
  var T = st.T, geo = phoenixStrip(T, hd ? 14 : 9), A = phoenixAttrs(T, geo, n);
  var mat = phoenixMat(st, hd, PHX_SHEET_VS, PHX_SCORCH_FS, phoenixWith({ uWide: 1, uFade: 0, uEmber: 1, uChar: new T.Color(PHX.char) }, clr));
  var mesh = new T.InstancedMesh(geo, mat, n), M = mesh.instanceMatrix.array, X = new T.Vector3(), Y = new T.Vector3(), Z = new T.Vector3();
  mesh.frustumCulled = false; mesh.renderOrder = 2;
  for (var i = 0; i < M.length; i++) M[i] = 0;
  return { mesh: mesh, mat: mat,
    brand: function (marks) {
      marks.forEach(function (mk, i) { phoenixPut(T, M, A, i, mk.root, mk.dir, 1, mk, X, Y, Z); });
      mesh.instanceMatrix.needsUpdate = true; A.aShape.needsUpdate = A.aFire.needsUpdate = A.aMode.needsUpdate = true;
    } };
}

function phoenixFloor(st, at, s, hd, clr) {
  var T = st.T, mat = phoenixMat(st, hd, PHX_FLOOR_VS, PHX_FLOOR_FS, phoenixWith({ uS: s, uShade: 0, uPool: 0, uRing: new T.Vector4(0, 0, 0, 0), uSpan: new T.Vector2(7, 4.2) }, clr));
  var m = new T.Mesh(new T.PlaneGeometry(s * 26, s * 26), mat);
  m.rotation.x = -Math.PI / 2; m.position.set(at.x, 0.008, at.z); m.renderOrder = 1; m.frustumCulled = false;
  return m;
}

// Embers (streaks stretched along their motion) or smoke (turning puffs that swell as they rise).
// fld pulls them into a vortex round a point while the bird spirals down.
function phoenixMotes(st, n, smoke, hd, rnd, clr) {
  var T = st.T, pos = new Float32Array(n * 12), uv = new Float32Array(n * 8), dat = new Float32Array(n * 16), idx = [];
  for (var i = 0; i < n; i++) { uv.set([0, 0, 1, 0, 0, 1, 1, 1], i * 8); var a = i * 4; idx.push(a, a + 1, a + 2, a + 1, a + 3, a + 2); }
  var geo = new T.BufferGeometry(), pa = new T.BufferAttribute(pos, 3), da = new T.BufferAttribute(dat, 4);
  pa.setUsage(T.DynamicDrawUsage); da.setUsage(T.DynamicDrawUsage);
  geo.setAttribute('position', pa); geo.setAttribute('aUv', new T.BufferAttribute(uv, 2)); geo.setAttribute('aDat', da); geo.setIndex(idx);
  var mat = phoenixMat(st, hd, PHX_MOTE_VS, smoke ? PHX_SMOKE_FS : PHX_EMBER_FS, phoenixWith(smoke ? { uMap: phoenixSmokeTex(T) } : { uGain: 1.2 }, clr));
  var mesh = new T.Mesh(geo, mat); mesh.frustumCulled = false; mesh.renderOrder = smoke ? 3 : 6;
  var P = new Float32Array(n * 3), V = new Float32Array(n * 3), age = new Float32Array(n), life = new Float32Array(n), sz = new Float32Array(n), ht = new Float32Array(n),
    sd = new Float32Array(n), dr = new Float32Array(n), by = new Float32Array(n), rot = new Float32Array(n), op = new Float32Array(n), next = 0;
  function quad(i, ax, az, bx, bz, cx, cz, dx, dz, y, h, al, sdv) {
    var o = i * 12, d = i * 16;
    pos[o] = ax; pos[o + 1] = y; pos[o + 2] = az; pos[o + 3] = bx; pos[o + 4] = y; pos[o + 5] = bz;
    pos[o + 6] = cx; pos[o + 7] = y; pos[o + 8] = cz; pos[o + 9] = dx; pos[o + 10] = y; pos[o + 11] = dz;
    for (var k = 0; k < 4; k++) { dat[d + k * 4] = h; dat[d + k * 4 + 1] = al; dat[d + k * 4 + 2] = sdv; dat[d + k * 4 + 3] = 0; }
  }
  return { mesh: mesh,
    emit: function (x, y, z, vx, vy, vz, lf, size, heat, drag, buoy, alpha) {
      var i = next; next = (next + 1) % n;
      P[i * 3] = x; P[i * 3 + 1] = y; P[i * 3 + 2] = z; V[i * 3] = vx; V[i * 3 + 1] = vy; V[i * 3 + 2] = vz;
      age[i] = 0; life[i] = lf; sz[i] = size; ht[i] = heat; sd[i] = rnd(); dr[i] = drag; by[i] = buoy; rot[i] = rnd() * 6.28; op[i] = alpha;
    },
    step: function (dt, time, fld) {
      for (var i = 0; i < n; i++) {
        if (age[i] >= life[i]) { quad(i, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0); continue; }
        age[i] += dt;
        var k = age[i] / life[i];
        if (k >= 1) { quad(i, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0); continue; }
        var j = i * 3, x = P[j], y = P[j + 1], z = P[j + 2], vx = V[j], vy = V[j + 1], vz = V[j + 2], tb = smoke ? 0.7 : 2.4;
        vy += by[i] * dt;
        vx += Math.sin(z * 1.7 + time * 2.3 + sd[i] * 9) * tb * dt; vz += Math.cos(x * 1.9 - time * 2.1 + sd[i] * 7) * tb * dt;
        if (fld.swirl || fld.pull) {
          var ex = x - fld.x, ez = z - fld.z, r = Math.sqrt(ex * ex + ez * ez) + 0.05;
          vx += (-ez / r * fld.swirl - ex * fld.pull) * dt; vz += (ex / r * fld.swirl - ez * fld.pull) * dt; vy += (fld.y - y) * fld.pull * dt;
        }
        var dd = Math.max(0, 1 - (dr[i] + fld.drag) * dt); vx *= dd; vy *= dd; vz *= dd;
        x += vx * dt; y += vy * dt; z += vz * dt; if (y < 0.03) { y = 0.03; vy = Math.abs(vy) * 0.2; }
        P[j] = x; P[j + 1] = y; P[j + 2] = z; V[j] = vx; V[j + 1] = vy; V[j + 2] = vz;
        if (smoke) {
          var h = sz[i] * (0.55 + 1.5 * k) * 0.5, an = rot[i] + time * 0.5 * (sd[i] - 0.5), c = Math.cos(an) * h, sn = Math.sin(an) * h;
          quad(i, x - c + sn, z - sn - c, x + c + sn, z + sn - c, x - c - sn, z - sn + c, x + c - sn, z + sn + c, y,
            ht[i] * (1 - k) * (1 - k), op[i] * smooth(0, 0.2, k) * Math.pow(1 - k, 1.3), sd[i]);
        } else {
          var sp = Math.sqrt(vx * vx + vz * vz), ux = 1, uz = 0; if (sp > 1e-4) { ux = vx / sp; uz = vz / sp; }
          var w = sz[i] * (1 - 0.55 * k), L = Math.min(Math.max(sp * 0.045, w), w * 9), nx = -uz * w * 0.5, nz = ux * w * 0.5;
          var hx = x + ux * w * 0.5, hz = z + uz * w * 0.5, tx = x - ux * L, tz = z - uz * L;
          quad(i, tx - nx, tz - nz, hx - nx, hz - nz, tx + nx, tz + nz, hx + nx, hz + nz, y, ht[i] * (1 - 0.7 * k), op[i] * Math.min(1, k * 8) * Math.pow(1 - k, 0.6), sd[i]);
        }
      }
      pa.needsUpdate = true; da.needsUpdate = true; mat.uniforms.uTime.value = time;
    } };
}

// Heat as colour, the way metal shows it: dull red, orange, gold, then white.
function phoenixHeat(h) {
  if (h <= 0) return '#000000';
  if (h < 0.3) return mix('#000000', PHX.red, h / 0.3);
  if (h < 0.6) return mix(PHX.red, PHX.orange, (h - 0.3) / 0.3);
  if (h < 0.9) return mix(PHX.orange, PHX.gold, (h - 0.6) / 0.3);
  return mix(PHX.gold, PHX.white, Math.min(1, (h - 0.9) / 0.35));
}
// The light that seeps into a face from its seams, as a white pattern: a dull glow across the face and
// its edges far brighter, since metal heats from the outside in. Tinted per heat step by multiplying a
// colour over it, so a redraw is cheap.
var PHX_SEAMS = {};
function phoenixSeamPattern(lay) {
  var key = lay.cv + ':' + lay.inner.map(function (p) { return Math.round(p[0]) + ',' + Math.round(p[1]); }).join(';');
  if (PHX_SEAMS[key]) return PHX_SEAMS[key];
  var cv = lay.cv, c = canvas(cv), g = c.getContext('2d');
  g.fillStyle = '#000'; g.fillRect(0, 0, cv, cv);
  g.beginPath(); lay.inner.forEach(function (p, k) { if (k) g.lineTo(p[0], p[1]); else g.moveTo(p[0], p[1]); }); g.closePath();
  g.fillStyle = 'rgb(120,120,120)'; g.fill();
  g.save(); g.clip(); g.lineJoin = 'round'; g.strokeStyle = '#fff';
  g.shadowColor = '#fff'; g.shadowBlur = cv * 0.09; g.lineWidth = cv * 0.12; g.stroke();
  g.shadowBlur = 0; g.lineWidth = cv * 0.05; g.stroke(); g.restore();
  PHX_SEAMS[key] = c; return c;
}
// One face's glow at a heat: the seam pattern in that heat's colour at strength a, and the number on top
// when asked. numA below 1 burns it in along its strokes like a fuse, over the number as it was lit (was).
function phoenixForge(gf, col, a, num, numA, was) {
  var g = gf.g, cv = gf.cv;
  g.globalCompositeOperation = 'source-over'; g.globalAlpha = 1; g.fillStyle = '#000'; g.fillRect(0, 0, cv, cv);
  if (a > 0) {
    g.globalAlpha = Math.min(1, a); g.drawImage(phoenixSeamPattern(gf.lay), 0, 0); g.globalAlpha = 1;
    g.globalCompositeOperation = 'multiply'; g.fillStyle = col; g.fillRect(0, 0, cv, cv); g.globalCompositeOperation = 'source-over';
  }
  if (num && numA < 1 && was) numberOnto(g, gf.lay, was);
  if (num && numA > 0) {
    if (numA < 1 && gf.lay.style !== 'classic') glowStrokes(g, cutPaths(gf.lay.strokes, numA), strokeW(gf.lay), num);
    else { g.globalAlpha = Math.min(1, numA); numberOnto(g, gf.lay, num); g.globalAlpha = 1; }
  }
}

// ---------- The effect ----------
function fxPhoenix(st, die, fi, th, o) {
  var T = st.T, g = die.group, s = die.size, t = 0, hd = !(st.q && st.q.detail === 'basic'), rnd = mulberry(4417), END = 4.8, Q = hd ? 1 : 0.45;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, at = new T.Vector3(p0.x, 0, p0.z), camY = st.cam.position.y;
  var NG = hd ? 26 : 14, NT = hd ? 7 : 5, NF = hd ? 11 : 7, NC = hd ? 8 : 5, NR = hd ? 3 : 2, NV = hd ? 12 : 7, NB = 5 + NR + 2;
  var iT0 = NG, iF0 = iT0 + NT, n = iF0 + 2 * NF + 2 * NC + NB + NV;
  // Dice to keep in view (o.clear): the wings, the scorch, the firelight, the smoke and the dim all leave a hole round each.
  var clear = o && o.clear || [], clr = clear.length ? phoenixSpots(st, clear, at, s * 0.9) : null;
  var sheets = phoenixSheets(st, n, hd, clr), embers = phoenixMotes(st, hd ? 640 : 280, false, hd, rnd), smoke = phoenixMotes(st, hd ? 72 : 34, true, hd, rnd, clr);
  var floor = phoenixFloor(st, at, s, hd, clr), dim = phoenixDimmer(st, at, s * 9, clr, !!clr), scorch = phoenixScorch(st, 2 * NF, hd, clr);
  var core = glow(st, spriteTex(T), PHX.gold, s * 4), star = glow(st, starTex(T), PHX.white, s * 11);
  core.renderOrder = star.renderOrder = 7;
  // One light, from the fire itself: it lights the die and moves with the bird. No shadow, as on any see-through table.
  var light = new T.PointLight(PHX.orange, 0, s * 16, 1.4);
  st.scene.add(floor, scorch.mesh, smoke.mesh, sheets.glow, sheets.flame, embers.mesh, core, star, light);

  // The die's seams, for the first tongues of flame. Only the sides: a tongue from the top face would
  // point at the camera and hide the 20.
  g.updateMatrixWorld(true);
  var edges = [], tongues = [], i;
  die.faces.forEach(function (fc, j) {
    var pts = fc.lay.poly.map(function (q) { return worldOf(die, faceToLocal(die, j, q[0], q[1])); });
    for (var k = 0; k < pts.length; k++) edges.push([pts[k], pts[(k + 1) % pts.length]]);
  });
  for (i = 0; i < NG; i++) {
    var e, root = new T.Vector3(), tries = 0;
    do { e = edges[Math.floor(rnd() * edges.length)]; root.copy(e[0]).lerp(e[1], 0.15 + rnd() * 0.7); tries++; }
    while ((root.y > y0 + s * 0.6 || Math.hypot(root.x - at.x, root.z - at.z) < s * 0.55) && tries < 16);
    tongues.push({ root: root, phi: Math.atan2(root.z - at.z, root.x - at.x) + (rnd() - 0.5) * 0.5, pitch: 0.12 + rnd() * 0.35, seed: rnd() * 10,
      rate: 1.3 + rnd() * 1.2, ph: rnd(), len: s * (0.8 + rnd() * 0.7), bend: (rnd() - 0.5) * 1.4 });
  }

  // The bird is posed in its own frame (x right, z back toward the tail, y up) round the point above the
  // die, then turned by yaw and scaled by rs about it: that is how it spirals down at the end.
  var R = { yaw: 0, rs: 1, H: s * 1.4, open: 0, sweep: 0, span: 1, lift: 0, curl: 0.1, heat: 1, alpha: 0, wgrow: 0, bgrow: 0, hgrow: 0, tgrow: 0, spin: 0, trail: 0 };
  var seeds = [], WL = [], TL = [], tips = new Float32Array(n * 4), W = new Float32Array(n * 8), BEND = new Float32Array(n);
  for (i = 0; i < n; i++) seeds.push(rnd() * 10);
  for (i = 0; i < NF; i++) WL.push(0.92 + rnd() * 0.16);
  for (i = 0; i < NT; i++) TL.push(0.88 + rnd() * 0.24);
  var f = { len: 0, wid: 0, bend: 0, lift: 0, seed: 0, heat: 0, alpha: 0, grow: 0, kind: 0, wave: 0, smoke: 0, flow: 1 };
  var wr = new T.Vector3(), wd = new T.Vector3(), EX = new Float32Array(17), EY = new Float32Array(17), EZ = new Float32Array(17), ex = 0, ey = 0, ez = 0;
  function show(i, sc) {
    if (f.alpha <= 0.003 || f.grow <= 0.003) { sheets.hide(i); tips[i * 4 + 3] = 0; return; }
    sheets.set(i, wr, wd, sc, f);
    var L = f.len * sc, k = i * 8, j = i * 4, reach = L * Math.min(1, f.grow) * 0.85;
    W[k] = wr.x; W[k + 1] = wr.y; W[k + 2] = wr.z; W[k + 3] = wd.x; W[k + 4] = wd.y; W[k + 5] = wd.z; W[k + 6] = L; W[k + 7] = f.wid * sc; BEND[i] = f.bend;
    tips[j] = wr.x + wd.x * reach; tips[j + 1] = wr.y + wd.y * reach; tips[j + 2] = wr.z + wd.z * reach; tips[j + 3] = f.alpha * Math.min(1, f.grow * 2);
  }
  function put(i, lx, ly, lz, phi, pitch) {
    var cy = Math.cos(R.yaw), sy = Math.sin(R.yaw), ph = phi + R.yaw, cp = Math.cos(pitch);
    wr.set(at.x + R.rs * (lx * cy - lz * sy), R.H + R.rs * ly, at.z + R.rs * (lx * sy + lz * cy));
    wd.set(Math.cos(ph) * cp, Math.sin(pitch), Math.sin(ph) * cp);
    f.seed = seeds[i]; show(i, R.rs);
  }
  // The right wing's leading edge, shoulder to tip; the left is its mirror.
  function wingEdge() {
    var arm = 1.3 + (-0.14 - 1.3) * R.open + R.sweep, curl = 0.15 + 0.65 * R.open, step = s * (0.9 + 2.0 * R.open) * R.span / 16, x = s * 0.42, z = -s * 0.15;
    EX[0] = x; EZ[0] = z; EY[0] = 0;
    for (var k = 1; k <= 16; k++) {
      var ang = arm + curl * Math.pow((k - 0.5) / 16, 1.6), a = k / 16;
      x += Math.cos(ang) * step; z += Math.sin(ang) * step;
      EX[k] = x; EZ[k] = z; EY[k] = s * (R.lift * 2.6 * Math.pow(a, 1.5) - 0.2 * a);
    }
  }
  function edgeAt(a) { var q = clamp01(a) * 16, k = Math.min(15, Math.floor(q)), w = q - k; ex = EX[k] + (EX[k + 1] - EX[k]) * w; ey = EY[k] + (EY[k + 1] - EY[k]) * w; ez = EZ[k] + (EZ[k + 1] - EZ[k]) * w; }
  // Feathers fan from straight back by the body to straight out at the tip; folded, they all lie back along it.
  function featherPhi(a) { var o = 1.62 + (0.1 - 1.62) * Math.pow(a, 1.5), c = 1.55 - 0.1 * a; return c + (o - c) * R.open + R.sweep * 0.55; }
  function tongueLevel(tt) { return Math.max(smooth(0.12, 0.55, tt) * (1 - smooth(1.15, 1.55, tt)), 0.75 * env4(tt, 3.36, 3.46, 3.7, 4.35)); }

  function build(tt) {
    var i = 0, k, a, sg, c;
    // Tongues licking out of the seams, each flaring and dying in a cycle of its own; they leap at the eruption.
    var lv = tongueLevel(tt), boost = env4(tt, 0.68, 0.84, 0.92, 1.3), hot = tt < 3 ? 0.88 : 0.85 - 0.4 * smooth(3.4, 4.3, tt);
    for (k = 0; k < NG; k++, i++) {
      var tg = tongues[k], cyc = tt * tg.rate + tg.ph, cc = cyc - Math.floor(cyc), sw = Math.sin(cc * Math.PI), cp = Math.cos(tg.pitch);
      f.alpha = lv * sw; f.grow = Math.min(1, cc * 2.6); f.len = tg.len * (0.6 + 0.6 * sw) * (1 + 1.6 * boost); f.wid = s * 0.55 * (1 + 0.4 * boost);
      f.bend = tg.bend; f.lift = 0.35; f.heat = hot * (1 + 0.15 * boost); f.kind = 1; f.wave = 0.35; f.smoke = 0.6; f.flow = 1.3; f.seed = tg.seed + Math.floor(cyc) * 1.37;
      wr.copy(tg.root); wd.set(Math.cos(tg.phi) * cp, Math.sin(tg.pitch), Math.sin(tg.phi) * cp);
      show(i, 1);
    }
    // The tail: long streamers fanned behind, rippling.
    for (k = 0; k < NT; k++, i++) {
      c = k / (NT - 1) * 2 - 1;
      f.len = s * (3.4 + 1.9 * (1 - c * c)) * TL[k] * (0.55 + 0.45 * R.tgrow); f.wid = s * 0.72; f.bend = c * 0.3 + R.trail * 1.4; f.lift = 0.04;
      f.heat = 0.84 * R.heat; f.alpha = R.alpha; f.grow = R.tgrow; f.kind = 2; f.wave = 0.2 + 0.3 * R.spin; f.smoke = 1; f.flow = 0.9;
      put(i, c * s * 0.25, -s * 0.1, s * 0.85, Math.PI / 2 + c * 0.42 + 0.07 * Math.sin(tt * 2.3 + k), -0.04);
    }
    // Flight feathers, outermost first so each inner one lies over the next one out, both wings together.
    wingEdge();
    for (k = NF - 1; k >= 0; k--) {
      a = 0.1 + 0.9 * k / (NF - 1); edgeAt(a);
      var pf = featherPhi(a) + 0.04 * Math.sin(tt * 7 + k * 1.7);
      for (sg = -1; sg <= 1; sg += 2, i++) {
        f.len = s * (2.5 + 1.15 * smooth(0.15, 1, a)) * WL[k] * (0.45 + 0.55 * R.open) * R.span; f.wid = s * (1.15 - 0.3 * a);
        f.bend = sg * (-(0.08 + 0.42 * a) - R.sweep * 0.5) + R.trail; f.lift = R.curl;
        f.heat = (0.86 - 0.16 * a) * R.heat; f.alpha = R.alpha; f.grow = clamp01((R.wgrow - a * 0.35) / 0.65); f.kind = 0; f.wave = 0.06; f.smoke = 1; f.flow = 1;
        put(i, sg * ex, ey, ez, sg > 0 ? pf : Math.PI - pf, R.lift * 0.3 * a);
      }
    }
    // Coverts: short, hotter feathers rooted just ahead of the edge, so the leading edge burns brightest.
    for (k = NC - 1; k >= 0; k--) {
      a = 0.04 + 0.84 * k / (NC - 1); edgeAt(a);
      var pc = featherPhi(a) + 0.12;
      for (sg = -1; sg <= 1; sg += 2, i++) {
        var pr = sg > 0 ? pc : Math.PI - pc;
        f.len = s * (1.25 + 0.8 * a) * (0.5 + 0.5 * R.open) * R.span; f.wid = s * 1.0; f.bend = -sg * 0.15 + R.trail; f.lift = R.curl * 0.6;
        f.heat = 0.98 * R.heat; f.alpha = R.alpha; f.grow = clamp01((R.wgrow - a * 0.3) / 0.6); f.kind = 0; f.wave = 0.04; f.smoke = 0.25; f.flow = 1.2;
        put(i, sg * ex - Math.cos(pr) * s * 0.15, ey + s * 0.05, ez - Math.sin(pr) * s * 0.15, pr, R.lift * 0.25 * a);
      }
    }
    // The body: two back plumes into the tail, a broad breast over the die, a crest streaming back off the
    // head, and the neck and head as tongues pointing forward, the head the hottest flame of all.
    var turn = 0.1 * Math.sin(tt * 1.6) * (1 - R.spin);
    for (k = 0; k < 2; k++, i++) {
      sg = k ? 1 : -1;
      f.len = s * 1.7; f.wid = s * 0.85; f.bend = sg * 0.15 + R.trail; f.lift = 0.05; f.heat = 0.88 * R.heat; f.alpha = R.alpha; f.grow = R.bgrow; f.kind = 0; f.wave = 0.08; f.smoke = 0.6; f.flow = 1;
      put(i, sg * s * 0.3, s * 0.05, s * 0.15, Math.PI / 2 - sg * 0.15, -0.05);
    }
    // The breast: a broad tongue whose wide base sits over the die (so the die never shows through as an
    // outline), flanked by two feathers.
    for (k = -1; k <= 1; k++, i++) {
      f.len = s * (k ? 2.1 : 2.5); f.wid = s * (k ? 1.3 : 2.3); f.bend = k * 0.22 + R.trail; f.lift = 0.08; f.heat = 0.96 * R.heat; f.alpha = R.alpha; f.grow = R.bgrow;
      f.kind = k ? 0 : 1; f.wave = 0.06; f.smoke = 0.35; f.flow = 1.1;
      put(i, k * s * 0.2, s * 0.2, -s * (k ? 0.7 : 0.95), Math.PI / 2 + k * 0.32 + turn * 0.3, -0.06);
    }
    for (k = 0; k < NR; k++, i++) {
      c = k / (NR - 1) * 2 - 1;
      f.len = s * (1.5 - 0.3 * Math.abs(c)); f.wid = s * 0.36; f.bend = c * 0.5 + R.trail; f.lift = 0.3; f.heat = R.heat; f.alpha = R.alpha; f.grow = R.hgrow; f.kind = 2; f.wave = 0.3; f.smoke = 0.5; f.flow = 1.2;
      put(i, c * s * 0.1 + turn * s, s * 0.55, -s * 1.5, Math.PI / 2 + c * 0.55, 0.15);
    }
    f.len = s * 2.3; f.wid = s * 1.45; f.bend = turn + R.trail * 0.5; f.lift = 0.06; f.heat = 1.0 * R.heat; f.alpha = R.alpha; f.grow = R.hgrow; f.kind = 1; f.wave = 0.08; f.smoke = 0; f.flow = 1.1;
    put(i, 0, s * 0.3, s * 0.65, -Math.PI / 2 + turn, 0.05); i++;
    f.len = s * 1.15; f.wid = s * 0.78; f.heat = 1.25 * R.heat;
    put(i, turn * s * 1.2, s * 0.45, -s * 0.85, -Math.PI / 2 + turn * 1.6, 0.05); i++;
    // The vortex: flame arms spiralling in round the die as the bird is drawn down into it.
    var vk = env4(tt, 2.7, 2.85, 3.22, 3.38), ck = smooth(2.7, 3.36, tt);
    for (k = 0; k < NV; k++, i++) {
      var a0 = k / NV * Math.PI * 2 + R.yaw * 1.1, pv = a0 + 0.3;
      wr.set(at.x + Math.cos(a0) * s * 0.35, Math.max(y0 + s * 0.9, R.H * 0.85), at.z + Math.sin(a0) * s * 0.35); wd.set(Math.cos(pv), 0, Math.sin(pv));
      f.len = s * (3.0 + 1.2 * ((k * 0.37) % 1)) * Math.pow(1 - ck, 0.7); f.wid = s * 0.62; f.bend = -1.9; f.lift = 0.05; f.heat = 0.9; f.alpha = 0.85 * vk;
      f.grow = smooth(2.7, 2.9, tt); f.kind = 1; f.wave = 0.25; f.smoke = 0.8; f.flow = 1.6; f.seed = seeds[i];
      show(i, 1);
    }
    sheets.commit(tt);
  }

  // A random live flame tip among the tail and wing feathers, or -1.
  function pickTip() {
    for (var k = 0; k < 6; k++) {
      var j = rnd() < 0.3 ? iT0 + Math.floor(rnd() * NT) : iF0 + Math.floor(rnd() * 2 * NF);
      if (tips[j * 4 + 3] > 0.2) return j;
    }
    return -1;
  }
  var acc = {};
  function every(key, rate, dt, fn) { acc[key] = (acc[key] || 0) + rate * Q * dt; while (acc[key] >= 1) { acc[key] -= 1; fn(); } }
  function burst(count, fn) { for (var k = Math.round(count * Q); k > 0; k--) fn(); }
  var FLD = { x: at.x, z: at.z, y: y0 + s * 0.8, swirl: 0, pull: 0, drag: 0 };

  // The die heats from its seams while the bird lives, goes forge-white as it takes the bird back, flashes,
  // and cools through gold, orange and red.
  function dieHeat(tt) {
    if (tt < 2.7) return 0.85 * smooth(0.12, 0.9, tt) - 0.5 * smooth(1.0, 1.4, tt);
    if (tt < 3.36) return 0.35 + 0.9 * smooth(2.7, 3.36, tt);
    return 1.25 * (1 - smooth(3.4, 4.6, tt)) + 0.35 * Math.max(0, 1 - (tt - 3.36) / 0.25);
  }
  var lastStep = -1, Yax = new T.Vector3(0, 1, 0), spinQ = new T.Quaternion(), done = {};
  // Faces are redrawn only when the heat moves a step, since each redraw uploads a texture per face.
  function forge(tt) {
    var h = dieHeat(tt), stp = Math.round(h / 0.1), fuse = clamp01((tt - 0.02) / 0.3);
    if (stp === lastStep && fuse >= 1) return;
    var top = glowFace(die, fi), hq = stp * 0.1, col = phoenixHeat(hq), a = Math.min(1, hq * 2), k = 0.7 + 1.5 * clamp01((hq - 0.4) / 0.8);
    phoenixForge(top, col, a * 0.45, hq > 0.3 ? PHX.white : PHX.gold, fuse, th.burn); top.commit(1.6);
    if (stp === lastStep) return;
    lastStep = stp;
    die.faces.forEach(function (fc, j) { if (j === fi) return; var gf = glowFace(die, j); phoenixForge(gf, col, a, null, 0); gf.commit(k); });
  }
  // Feather marks branded into the page by the downstroke: the flight feathers where they are now, laid
  // flat and pushed out a little, since from the camera a wing in the air looks bigger than its footprint.
  function brand() {
    var marks = [];
    for (var j = iF0; j < iF0 + 2 * NF; j++) {
      var k = j * 8; if (tips[j * 4 + 3] < 0.2) continue;
      var m = camY / Math.max(1, camY - W[k + 1]), dx = W[k + 3], dz = W[k + 5], hl = Math.hypot(dx, dz) || 1;
      marks.push({ root: new T.Vector3(at.x + (W[k] - at.x) * m, 0.012, at.z + (W[k + 2] - at.z) * m), dir: new T.Vector3(dx / hl, 0, dz / hl),
        len: W[k + 6] * hl * m * 0.95, wid: W[k + 7] * m * 0.9, bend: BEND[j], lift: 0, seed: seeds[j], heat: 0, alpha: 0.85, grow: 1, kind: 0, wave: 0, smoke: 0, flow: 0 });
    }
    scorch.brand(marks);
  }

  return { update: function (dt) {
    t += dt;
    // The pose: rise out of the die, unfurl, one beat (up, a fast stroke forward and down, recover), then
    // fold and spiral down into the die.
    var rise = out3((t - 0.75) / 0.75), up = smooth(1.72, 2.0, t), down = smooth(2.0, 2.2, t), rec = smooth(2.2, 2.7, t);
    var ck = smooth(2.7, 3.36, t), fold = smooth(2.66, 3.05, t), o1 = clamp01((t - 0.95) / 0.8);
    R.open = (out3(o1) + 0.06 * Math.sin(o1 * Math.PI)) * (1 - 0.8 * fold);
    R.lift = up - 1.6 * down + 0.6 * rec;
    R.sweep = 0.38 * up - 0.88 * down + 0.5 * rec + 0.35 * fold;
    R.span = 1 + 0.08 * up + 0.12 * down - 0.2 * rec;
    R.curl = 0.1 + 0.32 * R.lift;
    R.H = s * (1.4 + 1.25 * rise - 0.22 * up + 0.45 * down - 0.23 * rec); R.H += (s * 1.15 - R.H) * ck;
    var yaw = 7.5 * Math.pow(clamp01((t - 2.72) / 0.64), 2.2);
    R.spin = clamp01((yaw - R.yaw) / Math.max(dt, 1e-4) / 25); R.yaw = yaw; R.trail = -R.spin;
    R.rs = 1 - 0.93 * Math.pow(ck, 1.4); R.heat = 1 + 0.35 * ck;
    R.alpha = t < 0.74 ? 0 : 1 - smooth(3.22, 3.38, t);
    R.bgrow = smooth(0.76, 1.1, t); R.hgrow = smooth(0.88, 1.25, t); R.wgrow = smooth(0.92, 1.62, t); R.tgrow = smooth(0.98, 1.75, t);
    build(t);

    // Embers and smoke. emit(x, y, z, vx, vy, vz, life, size, heat, drag, buoyancy, alpha).
    var vt = env4(t, 2.7, 2.85, 3.3, 3.4); FLD.swirl = 14 * vt; FLD.pull = 26 * vt; FLD.drag = 2.2 * vt;
    every('k', t > 0.15 && t < 1.3 ? 70 : 0, dt, function () {
      var tg = tongues[Math.floor(rnd() * NG)], sp = 0.8 + rnd() * 1.6;
      embers.emit(tg.root.x, tg.root.y, tg.root.z, Math.cos(tg.phi) * sp + (rnd() - 0.5), 1.5 + rnd() * 2.5, Math.sin(tg.phi) * sp + (rnd() - 0.5), 0.6 + rnd() * 0.7, s * (0.08 + rnd() * 0.08), 0.75 + rnd() * 0.3, 0.8, 2.5, 1);
    });
    every('ks', t > 0.3 && t < 1.2 ? 14 : 0, dt, function () {
      var tg = tongues[Math.floor(rnd() * NG)];
      smoke.emit(tg.root.x, tg.root.y + s * 0.3, tg.root.z, Math.cos(tg.phi) * 0.5, 0.6 + rnd() * 0.6, Math.sin(tg.phi) * 0.5, 1.0 + rnd() * 0.3, s * (1.0 + rnd() * 0.4), 0.8, 0.5, 0.4, 0.35);
    });
    every('b', t > 1.0 && t < 2.75 ? 90 : 0, dt, function () {
      var j = pickTip(); if (j < 0) return;
      embers.emit(tips[j * 4], tips[j * 4 + 1], tips[j * 4 + 2], (rnd() - 0.5) * 1.5, 0.5 + rnd() * 1.5, 0.8 + rnd() * 1.2, 0.7 + rnd() * 0.7, s * (0.07 + rnd() * 0.07), 0.7 + rnd() * 0.3, 0.9, 1.6, 1);
    });
    every('bs', t > 1.2 && t < 2.7 ? 16 : 0, dt, function () {
      var j = pickTip(); if (j < 0) return;
      smoke.emit(tips[j * 4], tips[j * 4 + 1], tips[j * 4 + 2], (rnd() - 0.5) * 0.6, 0.3, 0.6, 1.0 + rnd() * 0.2, s * 1.3, 0.6, 0.6, 0.3, 0.25);
    });
    every('c', t > 2.7 && t < 3.28 ? 150 : 0, dt, function () {
      var j = pickTip(); if (j < 0) return;
      var x = tips[j * 4], z = tips[j * 4 + 2], dx = x - at.x, dz = z - at.z, r = Math.hypot(dx, dz) || 1, sp = 2 + rnd() * 2.5;
      embers.emit(x, tips[j * 4 + 1], z, -dz / r * sp, rnd(), dx / r * sp, 0.5 + rnd() * 0.4, s * (0.08 + rnd() * 0.06), 0.85 + rnd() * 0.2, 1.0, 0, 1);
    });
    every('cs', t > 2.75 && t < 3.25 ? 14 : 0, dt, function () {
      var j = pickTip(); if (j < 0) return;
      smoke.emit(tips[j * 4], tips[j * 4 + 1], tips[j * 4 + 2], 0, 0.2, 0, 0.8, s * 1.2, 0.7, 0.8, 0, 0.25);
    });
    every('a', t > 3.4 && t < 3.95 ? 26 : 0, dt, function () {
      var a = rnd() * 6.28, r = s * (0.3 + rnd() * 0.6);
      embers.emit(at.x + Math.cos(a) * r, y0 + s * (0.2 + rnd() * 0.8), at.z + Math.sin(a) * r, Math.cos(a) * 0.4, 0.8 + rnd() * 1.2, Math.sin(a) * 0.4, 0.6 + rnd() * 0.2, s * (0.06 + rnd() * 0.05), 0.55 + rnd() * 0.3, 0.6, 1.5, 1);
    });
    every('as', t > 3.38 && t < 3.7 ? 18 : 0, dt, function () {
      var a = rnd() * 6.28, r = s * rnd() * 0.6;
      smoke.emit(at.x + Math.cos(a) * r, y0 + s, at.z + Math.sin(a) * r, Math.cos(a) * 0.4, 0.7 + rnd() * 0.5, Math.sin(a) * 0.4, 0.9 + rnd() * 0.15, s * 0.9, 0.9, 0.6, 0.3, 0.3);
    });
    if (!done.erupt && t >= 0.8) {
      done.erupt = true;
      burst(70, function () { var a = rnd() * 6.28, sp = 1 + rnd() * 2.5; embers.emit(at.x + Math.cos(a) * s * 0.4, y0 + s * 0.9, at.z + Math.sin(a) * s * 0.4, Math.cos(a) * sp, 5 + rnd() * 6, Math.sin(a) * sp, 0.6 + rnd() * 0.6, s * (0.09 + rnd() * 0.08), 0.9 + rnd() * 0.15, 1.3, 0.5, 1); });
      burst(10, function () { var a = rnd() * 6.28; smoke.emit(at.x + Math.cos(a) * s * 0.8, y0 + s, at.z + Math.sin(a) * s * 0.8, Math.cos(a) * 1.5, 1 + rnd(), Math.sin(a) * 1.5, 1.1, s * 1.4, 1, 0.9, 0.3, 0.3); });
    }
    if (!done.beat && t >= 2.14) {
      done.beat = true; brand();
      // The downstroke flings sparks off along every feather.
      burst(170, function () {
        var j = iF0 + Math.floor(rnd() * 2 * NF), k = j * 8; if (tips[j * 4 + 3] < 0.2) return;
        var L = W[k + 6] * (0.3 + rnd() * 0.7), sp = 4 + rnd() * 5;
        embers.emit(W[k] + W[k + 3] * L, W[k + 1] + W[k + 4] * L, W[k + 2] + W[k + 5] * L, W[k + 3] * sp, (rnd() - 0.5) * 2, W[k + 5] * sp - 1.5, 0.6 + rnd() * 0.7, s * (0.08 + rnd() * 0.08), 0.8 + rnd() * 0.25, 1.7, 0.8, 1);
      });
      burst(14, function () {
        var j = iF0 + Math.floor(rnd() * 2 * NF), k = j * 8, L = W[k + 6] * rnd();
        smoke.emit(W[k] + W[k + 3] * L, W[k + 1], W[k + 2] + W[k + 5] * L, W[k + 3] * 1.5, 0.2, W[k + 5] * 1.5, 1.2, s * 1.6, 0.8, 0.8, 0.2, 0.3);
      });
    }
    if (!done.flash && t >= 3.36) {
      done.flash = true;
      burst(80, function () { var a = rnd() * 6.28, sp = 4 + rnd() * 5; embers.emit(at.x, y0 + s * 0.8, at.z, Math.cos(a) * sp, 1 + rnd() * 3, Math.sin(a) * sp, 0.4 + rnd() * 0.5, s * (0.08 + rnd() * 0.08), 0.95 + rnd() * 0.2, 2.5, 2, 1); });
    }
    embers.step(dt, t, FLD); smoke.step(dt, t, FLD);

    // The page: dimmed under the bird, a flickering pool of firelight, a ring of heat-light thrown out by
    // the eruption, the wingbeat and the flash, and the scorch marks cooling and fading.
    var fu = floor.material.uniforms, rg = fu.uRing.value, k1 = clamp01((t - 2.12) / 0.95), ke = clamp01((t - 0.78) / 0.6), kf = clamp01((t - 3.36) / 0.64);
    var fade = 1 - smooth(3.6, 4.4, t);
    fu.uTime.value = t;
    fu.uShade.value = 0.42 * env4(t, 0.7, 1.4, 3.0, 3.6);
    fu.uPool.value = (0.12 * smooth(0.15, 0.8, t) + 0.06 * env4(t, 0.9, 1.3, 2.8, 3.4) + 0.3 * env4(t, 2.05, 2.15, 2.25, 2.7)) * fade;
    rg.x = 3 + 7 * out3(k1); rg.y = 0.5 * smooth(2.12, 2.24, t) * Math.pow(1 - k1, 1.5);
    if (t >= 3.36) { rg.z = 0.8 + 6.2 * out3(kf); rg.w = 0.65 * (1 - kf) * (1 - kf); }
    else { rg.z = 0.8 + 4.5 * out3(ke); rg.w = t >= 0.78 ? 0.6 * (1 - ke) * (1 - ke) : 0; }
    var su = scorch.mat.uniforms; su.uTime.value = t; su.uFade.value = smooth(2.14, 2.3, t) * (1 - smooth(3.6, 4.5, t)); su.uEmber.value = 1 - smooth(2.2, 3.1, t);
    dim.material.opacity = 0.64 * env4(t, 0.05, 0.8, 3.55, 4.6);

    // Glow at the heart of the fire, and a star at the eruption and at the flash.
    core.position.set(at.x, t < 0.8 || t > 3.36 ? y0 + s : Math.max(R.H, y0 + s), at.z);
    core.material.opacity = (0.32 * smooth(0.1, 0.7, t) * (1 - 0.6 * env4(t, 1.0, 1.4, 2.8, 3.1)) + 0.25 * env4(t, 0.75, 0.9, 1.0, 1.4) + 0.4 * env4(t, 2.9, 3.3, 3.4, 3.7)) * fade;
    core.scale.setScalar(s * (2.6 + 1.6 * env4(t, 0.75, 0.88, 0.95, 1.4) + 1.2 * env4(t, 3.2, 3.36, 3.4, 3.8)));
    var big = t >= 3.36, sk = big ? (t - 3.36) / 0.55 : (t - 0.78) / 0.45;
    star.position.set(at.x, y0 + s * 1.2, at.z);
    star.material.opacity = sk >= 0 && sk < 1 ? (big ? 0.9 : 0.4) * (1 - sk) * (1 - sk) : 0;
    star.material.rotation = sk * 0.6; star.scale.setScalar(s * (6 + 8 * out3(sk)) * (big ? 1 : 0.7));
    var fl = 0.85 + 0.12 * Math.sin(t * 29) + 0.08 * Math.sin(t * 47 + 1);
    light.position.set(at.x, t < 0.75 || t > 3.36 ? y0 + s * 1.3 : R.H + s * 0.4, at.z);
    light.intensity = (14 * smooth(0.1, 0.7, t) + 30 * env4(t, 0.75, 0.85, 0.95, 1.4) + 10 * env4(t, 0.9, 1.3, 2.9, 3.3) + 40 * env4(t, 2.05, 2.15, 2.2, 2.6) + 60 * env4(t, 3.3, 3.36, 3.4, 3.9)) * fade * fl;

    // The die turns once with the vortex, lifts a little and comes down as it takes the bird in. One full
    // turn, so it lands exactly as it lay.
    var dk = clamp01((t - 2.9) / 0.46);
    if (dk > 0 && dk < 1) { g.quaternion.copy(spinQ.setFromAxisAngle(Yax, -Math.PI * 2 * inout(dk))).multiply(q0); g.position.set(p0.x, y0 + s * 0.3 * Math.sin(Math.PI * dk), p0.z); }
    else { g.quaternion.copy(q0); g.position.copy(p0); }
    forge(t);
    var h = dieHeat(t);
    tintDie(die, phoenixHeat(Math.min(1.35, h + 0.15)), 1.6 * Math.min(1, h * 1.5) * fl * (1 - 0.75 * env4(t, 1.0, 1.4, 2.7, 3.0)));
    shadeDie(die, 1 - 0.55 * smooth(0.1, 0.8, t) * (1 - smooth(3.7, 4.65, t)));

    if (t >= END) {
      drop(st, [sheets.glow, sheets.flame, floor, scorch.mesh, embers.mesh, smoke.mesh, core, star, dim]);
      sheets.flame.dispose(); sheets.glow.dispose(); scorch.mesh.dispose();
      st.scene.remove(light); light.dispose();
      g.position.copy(p0); g.quaternion.copy(q0);
      die.faces.forEach(function (fc, j) { var gf = glowFace(die, j); gf.clear(); gf.commit(1); });
      glowNumeral(die, fi, PHX.gold); shadeDie(die, 1); tintDie(die, th.burn, 0);
      return false;
    }
    return true;
  } };
}

// ---------- Echo: one feather ----------
// One flame feather leaves the grand bird's wing (or, with no grand die, flares out of this die),
// circles the die once, and burns away from the tip, shedding embers that cool on the page. It draws
// above anything the grand put on the page (render order 60 up, no depth test). The die's top face
// heats gold and the 20 is left burning.
function fxPhoenixEcho(st, die, fi, th, e) {
  var T = st.T, g = die.group, s = die.size, hd = !(st.q && st.q.detail === 'basic'), t = 0, rnd = mulberry(8123 + Math.floor(Math.random() * 1e6)), Q = hd ? 1 : 0.5, END = 1.9;
  var p0 = g.position.clone(), q0 = g.quaternion.clone(), y0 = p0.y, at = new T.Vector3(p0.x, 0, p0.z), FROM = !!(e && e.from), topY = worldOf(die, die.faces[fi].c).y;
  var Rc = s * 1.65, Lf = s * 2.6, Hc = y0 + s * 0.75, TF = FROM ? 0.45 : 0.4, TO = 0.9, PHI = 6.2832, W0 = PHI * 1.5 / TO;
  var sheets = phoenixSheets(st, 1, hd), embers = phoenixMotes(st, hd ? 150 : 70, false, hd, rnd), smoke = phoenixMotes(st, hd ? 12 : 6, true, hd, rnd);
  var dim = dimmer(st, at, s * 3), core = glow(st, spriteTex(T), PHX.gold, s * 1.5);
  sheets.glow.renderOrder = 62; sheets.flame.renderOrder = 63; smoke.mesh.renderOrder = 61; embers.mesh.renderOrder = 66; core.renderOrder = 64;
  [sheets.glow, sheets.flame, smoke.mesh, embers.mesh].forEach(function (m) { m.material.depthTest = false; }); core.material.depthTest = false;
  st.scene.add(smoke.mesh, sheets.glow, sheets.flame, embers.mesh, core);

  // Where it comes from, and the point on its circle nearest that.
  var S = new T.Vector3(), th0 = rnd() * 6.283;
  if (FROM) { S.copy(worldOf(e.from, e.from.faces[e.fromFi].c)); S.y += s * 1.4; th0 = Math.atan2(S.z - at.z, S.x - at.x); }
  var E = new T.Vector3(at.x + Math.cos(th0) * Rc, Hc, at.z + Math.sin(th0) * Rc), C = new T.Vector3(), dSE = S.distanceTo(E);
  // The flight bends in so that it arrives along the circle, not across it.
  C.set(E.x + Math.sin(th0) * dSE * 0.4, (S.y + E.y) / 2 + s * 0.8, E.z - Math.cos(th0) * dSE * 0.4);
  // Flight (or, alone, a spiral out of the die), then one turn of the circle that slows to a stop.
  function pathAt(tau, out) {
    if (tau < 0) tau = 0;
    if (tau < TF) {
      var u = tau / TF;
      if (FROM) { var a = (1 - u) * (1 - u), b = 2 * (1 - u) * u, c = u * u; return out.set(S.x * a + C.x * b + E.x * c, S.y * a + C.y * b + E.y * c, S.z * a + C.z * b + E.z * c); }
      var k = out3(u), r = Rc * (0.18 + 0.82 * k), an = th0 - W0 * (TF - tau);
      return out.set(at.x + Math.cos(an) * r, topY + (Hc - topY) * k, at.z + Math.sin(an) * r);
    }
    var v = clamp01((tau - TF) / TO), an2 = th0 + PHI * (1 - Math.pow(1 - v, 1.5));
    return out.set(at.x + Math.cos(an2) * Rc, Hc, at.z + Math.sin(an2) * Rc);
  }
  var P1 = new T.Vector3(), P2 = new T.Vector3(), P3 = new T.Vector3(), EP = new T.Vector3(), dirv = new T.Vector3(1, 0, 0), bend = 0;
  // The feather trails behind its root, bent along the path it has flown.
  function pose(tt) {
    pathAt(tt, P1); pathAt(tt + 0.02, P2); pathAt(tt + 0.04, P3);
    var ax = P2.x - P1.x, ay = P2.y - P1.y, az = P2.z - P1.z, l = Math.sqrt(ax * ax + ay * ay + az * az), bx = P3.x - P2.x, bz = P3.z - P2.z, hl = Math.hypot(ax, az);
    if (l < 1e-4 || hl < 1e-4) return;
    dirv.set(-ax / l, -ay / l, -az / l);
    bend = Math.max(-2.4, Math.min(2.4, -Math.atan2(ax * bz - az * bx, ax * bx + az * bz) / hl * Lf));
  }
  var f = { len: Lf, wid: s * 1.05, bend: 0, lift: 0.1, seed: rnd() * 10, heat: 0.95, alpha: 1, grow: 0, kind: 0, wave: 0.12, smoke: 0.8, flow: 1.2 };
  var acc = {}, FLD = { x: at.x, z: at.z, y: y0, swirl: 0, pull: 0, drag: 0 };
  function every(key, rate, dt, fn) { acc[key] = (acc[key] || 0) + rate * Q * dt; while (acc[key] >= 1) { acc[key] -= 1; fn(); } }
  // A point u of the way along the feather (it follows the path its root has flown).
  function along(u) { return pathAt(t - u * Lf / 12, EP); }

  // The top face heats from its seams; steps of a tenth keep the redraws few.
  var top = glowFace(die, fi), lastStep = -1, Yq = 0;
  function dieHeat(tt) { return 0.62 * smooth(0.1, 0.6, tt) * (1 - smooth(1.2, 1.85, tt)); }
  function forge(tt) {
    var stp = Math.round(dieHeat(tt) / 0.1); if (stp === lastStep) return; lastStep = stp;
    var hq = stp * 0.1; phoenixForge(top, phoenixHeat(hq), Math.min(1, hq * 2) * 0.45, hq > 0.3 ? PHX.white : PHX.gold, 1, th.burn); top.commit(1.5);
  }
  var lit = false;
  return { update: function (dt) {
    t += dt;
    if (t >= END) {
      drop(st, [sheets.glow, sheets.flame, embers.mesh, smoke.mesh, core, dim]);
      sheets.flame.dispose(); sheets.glow.dispose();
      g.position.copy(p0); g.quaternion.copy(q0);
      glowNumeral(die, fi, PHX.gold); shadeDie(die, 1); tintDie(die, th.burn, 0);
      return false;
    }
    pose(t);
    f.grow = smooth(0, 0.25, t) * (1 - smooth(1.05, 1.62, t)); f.alpha = 1 - smooth(1.5, 1.72, t); f.bend = bend;
    if (f.grow > 0.01 && f.alpha > 0.01) sheets.set(0, P1, dirv, 1, f); else sheets.hide(0);
    sheets.commit(t);

    // A first burst where it is born, embers trailing along it, then the burn at its tip: embers thrown
    // out over the page and cooling, and a little smoke.
    if (!lit) { lit = true; var b0 = FROM ? S : P1; for (var k = Math.round(14 * Q); k > 0; k--) { var a0 = rnd() * 6.283, sp = 1 + rnd() * 2; embers.emit(b0.x, b0.y, b0.z, Math.cos(a0) * sp, 1 + rnd() * 2.5, Math.sin(a0) * sp, 0.4 + rnd() * 0.4, s * (0.07 + rnd() * 0.06), 0.9, 1.2, -0.5, 1); } }
    every('t', t > 0.05 && t < 1.3 ? 55 : 0, dt, function () {
      along(0.15 + rnd() * 0.85);
      embers.emit(EP.x, EP.y, EP.z, (rnd() - 0.5) * 1.2, 0.2 + rnd() * 0.8, (rnd() - 0.5) * 1.2, 0.5 + rnd() * 0.4, s * (0.06 + rnd() * 0.06), 0.7 + rnd() * 0.3, 0.9, -1.4, 1);
    });
    every('b', t > 1.05 && t < 1.62 ? 170 : 0, dt, function () {
      along(clamp01(f.grow * 1.15 - 0.08) * (0.7 + rnd() * 0.3) + 0.04);
      var dx = EP.x - at.x, dz = EP.z - at.z, dl = Math.hypot(dx, dz) || 1, v = 0.8 + rnd() * 1.8;
      embers.emit(EP.x, EP.y, EP.z, dx / dl * v - dz / dl * 0.8, 0.3 + rnd() * 1.2, dz / dl * v + dx / dl * 0.8, 0.45 + rnd() * 0.25, s * (0.06 + rnd() * 0.07), 0.8 + rnd() * 0.2, 1.0, -2.4, 1);
    });
    every('s', t > 1.1 && t < 1.5 ? 12 : 0, dt, function () { along(rnd()); smoke.emit(EP.x, EP.y, EP.z, (rnd() - 0.5) * 0.6, 0.3, (rnd() - 0.5) * 0.6, 0.7, s * 0.8, 0.7, 0.8, 0.2, 0.28); });
    embers.step(dt, t, FLD); smoke.step(dt, t, FLD);

    core.position.copy(P1); core.material.opacity = 0.22 * smooth(0, 0.2, t) * (1 - smooth(1.05, 1.6, t)); core.visible = core.material.opacity > 0.003;
    dim.material.opacity = 0.42 * smooth(0, 0.25, t) * (1 - smooth(1.2, 1.9, t));
    forge(t);
    var h = dieHeat(t), fl = 0.88 + 0.12 * Math.sin(t * 29);
    tintDie(die, phoenixHeat(Math.min(1, h + 0.15)), 1.1 * Math.min(1, h * 1.5) * fl);
    shadeDie(die, 1 - 0.25 * smooth(0.1, 0.6, t) * (1 - smooth(1.4, 1.9, t)));
    return true;
  } };
} registerCrit('phoenix', { name: 'Phoenix', kind: 'succ', what: 'The 20 catches fire and a phoenix of flame bursts out of the die, spreads its wings across the page and beats them once in a storm of embers, then spirals back down into the die, which glows like hot metal with the 20 burning gold.', fx: fxPhoenix,
  // beat: the wingbeat that flings the feathers; peak: the bird has flashed back into the die.
  beat: 2.14, peak: 3.7, echo: fxPhoenixEcho, echoWhat: 'One flame feather circles the die and falls as embers.' });
