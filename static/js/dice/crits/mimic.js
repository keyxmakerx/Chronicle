// ---------- Crit fail: Mimic ----------
// It was never a die. It shudders, an eye opens on its top, a ring of teeth pushes out all round it,
// and a long tongue lashes across the page, sweeps, and snaps back. It bites twice at nothing, then
// folds its teeth away, shuts its eye, and sits there as if nothing happened, the 1 glowing.
import {
  dimmer, glowNumeral, in3, out3, registerCrit, shadeDie, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene } from './kit.js';

var MIMIC_RED = '#ff4a5a', MIMIC_HIDE = '#7a2b8a';
// The eye is drawn by its own shader, so the lids can open and the pupil can look about.
var MIMIC_EYE_FS = [
  'uniform float uOpen, uT; uniform vec2 uLook; varying vec2 vUv;',
  'void main() {',
  '  vec2 p = vUv * 2.0 - 1.0;',
  // Two arcs make the lids; uOpen is how far apart they stand.
  '  float lid = abs( p.y ) / max( uOpen, 0.001 ) + p.x * p.x;',
  '  float a = 1.0 - smoothstep( 0.88, 1.0, lid );',
  '  if ( a <= 0.003 ) discard;',
  '  vec2 q = p - uLook; float r = length( q );',
  '  vec3 col = mix( vec3( 1.0, 0.86, 0.2 ), vec3( 0.86, 0.3, 0.05 ), smoothstep( 0.1, 0.62, r ) );',
  '  col *= 0.8 + 0.2 * sin( atan( q.y, q.x ) * 24.0 );',
  '  col = mix( col, vec3( 0.96, 0.9, 0.82 ), smoothstep( 0.6, 0.68, r ) );',
  // A slit pupil, thin in the light.
  '  float pup = 1.0 - smoothstep( 0.07, 0.1, abs( q.x ) + q.y * q.y * 0.42 );',
  '  col = mix( col, vec3( 0.02, 0.0, 0.03 ), pup );',
  '  col += vec3( 1.0 ) * ( 1.0 - smoothstep( 0.02, 0.08, length( q - vec2( -0.16, 0.2 ) ) ) ) * 0.8;',
  // The lids shade the rim, and the rim is raw.
  '  col = mix( col, vec3( 0.45, 0.05, 0.12 ), smoothstep( 0.6, 1.0, lid ) );',
  '  gl_FragColor = vec4( col, a );',
  '  #include <colorspace_fragment>',
  '}'].join('\n');
function mimicEye(T, x, size) {
  var mat = new T.ShaderMaterial({ uniforms: { uOpen: { value: 0 }, uT: { value: 0 }, uLook: { value: new T.Vector2() } }, transparent: true, depthWrite: false,
    vertexShader: 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }', fragmentShader: MIMIC_EYE_FS });
  var m = new T.Mesh(new T.PlaneGeometry(size, size), mat); m.rotation.x = -Math.PI / 2; m.position.set(x.c.x, x.p0.y + x.s * 1.02, x.c.z); m.renderOrder = 12;
  return x.add(m);
}
// A tube that follows a path: the tongue. follow(points, shown) redraws it along the first part of the path.
function mimicTube(T, x, radius) {
  var SEG = 40, SID = 8, pos = new Float32Array((SEG + 1) * (SID + 1) * 3), nrm = new Float32Array(pos.length), idx = [];
  for (var k = 0; k < SEG; k++) for (var j = 0; j < SID; j++) { var a = k * (SID + 1) + j, b = a + SID + 1; idx.push(a, b, a + 1, a + 1, b, b + 1); }
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.BufferAttribute(pos, 3)); geo.setAttribute('normal', new T.BufferAttribute(nrm, 3)); geo.setIndex(idx);
  var mat = new T.MeshStandardMaterial({ color: '#e8507c', roughness: 0.55, metalness: 0, side: T.DoubleSide, emissive: new T.Color('#b0284e'), emissiveIntensity: 0.75 });
  var m = new T.Mesh(geo, mat); m.frustumCulled = false; m.castShadow = true; x.add(m);
  var curve = new T.CatmullRomCurve3([new T.Vector3(), new T.Vector3(0, 1, 0)]), P = new T.Vector3(), Tn = new T.Vector3(), N = new T.Vector3(), B = new T.Vector3(), up = new T.Vector3(0, 1, 0);
  return { mesh: m, follow: function (pts, shown) {
    m.visible = shown > 0.01; if (!m.visible) return;
    curve.points = pts;
    for (var k = 0; k <= SEG; k++) {
      var u = k / SEG * shown; curve.getPoint(u, P); curve.getTangent(u, Tn);
      N.crossVectors(up, Tn).normalize(); B.crossVectors(Tn, N).normalize();
      // Thick at the root, a bulb at the tip, flat like a tongue.
      var w = k / SEG, r = radius * (1 - 0.55 * w + 0.5 * Math.exp(-Math.pow((w - 0.94) * 9, 2))) * (k === SEG ? 0.3 : 1);
      for (var j = 0; j <= SID; j++) {
        var an = j / SID * 6.2832, cx = Math.cos(an) * 1.5, cy = Math.sin(an) * 0.55, o = (k * (SID + 1) + j) * 3;
        pos[o] = P.x + (N.x * cx + B.x * cy) * r; pos[o + 1] = P.y + (N.y * cx + B.y * cy) * r; pos[o + 2] = P.z + (N.z * cx + B.z * cy) * r;
        nrm[o] = N.x * cx * 0.4 + B.x * cy * 2; nrm[o + 1] = N.y * cx * 0.4 + B.y * cy * 2; nrm[o + 2] = N.z * cx * 0.4 + B.z * cy * 2;
      }
    }
    geo.attributes.position.needsUpdate = true; geo.attributes.normal.needsUpdate = true;
  } };
}

function fxMimic(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.0, hd = x.hd, rnd = x.rnd;
  var dim = dimmer(st, c, 8 * s); x.own.push(dim);
  var eye = mimicEye(T, x, 1.5 * s), EU = eye.material.uniforms;
  // Teeth: a ring of fangs round the die, each on a hinge so the ring can bite.
  var NT = hd ? 16 : 10, tg = new T.ConeGeometry(0.15 * s, 0.74 * s, 6); tg.translate(0, 0.37 * s, 0);
  var teeth = new T.InstancedMesh(tg, new T.MeshStandardMaterial({ color: '#f4ead0', roughness: 0.35, metalness: 0, emissive: new T.Color('#3a2a10'), emissiveIntensity: 0.5 }), NT);
  teeth.frustumCulled = false; teeth.castShadow = true; x.add(teeth);
  var m4 = new T.Matrix4(), q = new T.Quaternion(), e = new T.Euler(0, 0, 0, 'YXZ'), v3 = new T.Vector3(), sc = new T.Vector3();
  var tongue = mimicTube(T, x, 0.2 * s), aim = Math.atan2(-c.z, -c.x) + (rnd() - 0.5) * 0.6, pts = [0, 1, 2, 3, 4, 5].map(function () { return new T.Vector3(); });
  var done = {}, lit = false;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.45 * smooth(0.3, 1.0, t) * (1 - smooth(4.2, 4.9, t));
    // First a shudder, as something inside wakes.
    var wake = smooth(0.1, 0.6, t) * (1 - smooth(0.7, 1.0, t)), bite = Math.pow(Math.max(0, Math.sin(Math.PI * (t - 3.0) / 0.34)), 2) * (t > 3.0 && t < 3.68 ? 1 : 0), jit = s * 0.05 * wake;
    x.g.position.set(x.p0.x + (rnd() - 0.5) * jit, x.p0.y, x.p0.z + (rnd() - 0.5) * jit);
    x.g.scale.setScalar(1 + 0.05 * Math.sin(t * 7) * smooth(0.9, 1.3, t) * (1 - smooth(3.8, 4.3, t)) + 0.1 * bite);
    var alive = smooth(0.8, 1.2, t) * (1 - smooth(4.0, 4.6, t));
    shadeDie(die, 1 - 0.3 * alive); tintDie(die, MIMIC_HIDE, 0.6 * alive);
    // The eye: opens, looks about, narrows at each bite, shuts.
    EU.uOpen.value = 0.62 * out3((t - 0.85) / 0.3) * (1 - 0.7 * bite) * (1 - smooth(3.9, 4.25, t)) * (1 - 0.9 * Math.pow(Math.max(0, Math.sin(Math.PI * (t - 1.5) / 0.16)), 2) * (t > 1.5 && t < 1.66 ? 1 : 0));
    EU.uLook.value.set(0.22 * Math.sin(t * 2.3), 0.14 * Math.sin(t * 3.1 + 1)); eye.visible = EU.uOpen.value > 0.01;
    // The teeth push out, lie open, snap shut twice, and draw back in.
    var out = out3((t - 1.0) / 0.35) * (1 - smooth(3.9, 4.3, t));
    for (var i = 0; i < NT; i++) {
      var a = i / NT * 6.2832, grow = Math.max(0, Math.min(1, out * 1.4 - (i % 4) * 0.1)), tilt = 1.25 - 1.0 * bite;
      e.set(0, -a, -tilt); q.setFromEuler(e); sc.setScalar(Math.max(0.001, grow) * (i % 2 ? 0.8 : 1.1));
      v3.set(c.x + Math.cos(a) * s * 0.86, x.p0.y + s * 0.2, c.z + Math.sin(a) * s * 0.86); m4.compose(v3, q, sc); teeth.setMatrixAt(i, m4);
    }
    teeth.instanceMatrix.needsUpdate = true;
    // The tongue: out fast, a slow sweep across the page, back faster.
    var reach = out3((t - 1.7) / 0.3) * (1 - in3((t - 2.65) / 0.3)), sweep = aim + 1.1 * Math.sin((t - 1.7) * 2.6) * smooth(1.9, 2.2, t), L = 5.2 * s;
    for (var k = 0; k < 6; k++) {
      var u = k / 5, bend = sweep + Math.sin(u * 3 - t * 6) * 0.25 * u + u * u * 0.9 * Math.sin(t * 3.3);
      pts[k].set(c.x + Math.cos(bend) * L * u, x.p0.y + s * (0.6 - 0.5 * u) + Math.sin(u * Math.PI) * 0.7 * s, c.z + Math.sin(bend) * L * u);
    }
    tongue.follow(pts, reach);
    if (t >= 1.72) once('lash', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, MIMIC_RED, s * 0.5, s * 4, 0.7));
      st.fx.push(spray(st, { n: hd ? 60 : 26, colors: ['#d8f0a0', '#f4ffd8', '#a8d86a'], size: s * 0.06, life: [0.5, 1.1], drag: 0.8, g: 3, blend: 'normal',
        spawn: function (j, r) { var a = aim + (r() - 0.5) * 1.6, v = 2 + r() * 4; return { p: [c.x, x.p0.y + 0.5 * s, c.z], v: [Math.cos(a) * v, 0.6 + r() * 1.4, Math.sin(a) * v], delay: r() * 0.9 }; } }));
    });
    [3.17, 3.51].forEach(function (bt, i) { if (t >= bt) once('bite' + i, function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, '#ffd0b8', s * 0.9, s * 2.4, 0.35));
      st.fx.push(spray(st, { n: hd ? 30 : 14, colors: ['#fff3c4', '#ffffff'], size: s * 0.05, life: [0.2, 0.5], drag: 1.6, g: 2,
        spawn: function (j, r) { var a = r() * 6.28, v = 1.5 + r() * 3; return { p: [c.x + Math.cos(a) * s * 0.6, x.p0.y + s * 0.8, c.z + Math.sin(a) * s * 0.6], v: [Math.cos(a) * v, 0.5 + r() * 1.5, Math.sin(a) * v] }; } }));
    }); });
    if (t >= 4.3 && !lit) { lit = true; glowNumeral(die, fi, MIMIC_RED); }
    if (t >= END) { teeth.dispose(); x.end(MIMIC_RED); return false; }
    return true;
  } };
}
// An eye opens on the die, looks about, blinks and shuts.
function fxMimicEcho(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, t = 0, END = 2.0, lit = false, eye = mimicEye(T, x, 1.3 * s), EU = eye.material.uniforms, ph = x.rnd() * 6;
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var blink = Math.pow(Math.max(0, Math.sin(Math.PI * (t - 0.9) / 0.16)), 2) * (t > 0.9 && t < 1.06 ? 1 : 0);
    EU.uOpen.value = 0.6 * out3((t - 0.1) / 0.25) * (1 - 0.9 * blink) * (1 - smooth(1.5, 1.8, t)); EU.uLook.value.set(0.24 * Math.sin(t * 4 + ph), 0.12 * Math.cos(t * 3 + ph)); eye.visible = EU.uOpen.value > 0.01;
    var alive = smooth(0.1, 0.4, t) * (1 - smooth(1.5, 1.9, t)); shadeDie(die, 1 - 0.3 * alive); tintDie(die, MIMIC_HIDE, 0.6 * alive);
    if (t >= 1.75 && !lit) { lit = true; glowNumeral(die, fi, MIMIC_RED); }
    if (t >= END) { x.end(MIMIC_RED); return false; }
    return true;
  } };
}
registerCrit('mimic', { name: 'Mimic', kind: 'fail', what: 'The die was a mimic all along: an eye opens on top of it, a ring of teeth pushes out all round, and a long tongue lashes across the page before it bites twice and settles.', fx: fxMimic,
  beat: 1.72, peak: 3.7, echo: fxMimicEcho, echoWhat: 'An eye opens on each other 1, looks about, blinks and shuts.' });
