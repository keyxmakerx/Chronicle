// ---------- Crit fail: Meltdown ----------
// The die runs too hot and loses its shape. It glows, slumps and sinks into a spreading pool of its
// own colour until only the number is left, floating on the melt. Then the pool draws back in, the die
// stands up out of it, and cools.
import {
  dimmer, glowNumeral, isLight, mix, out3, registerCrit, shadeDie, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, flat, numeral, softTex } from './kit.js';

var MELT_HOT = '#ff7a1a', MELT_WHITE = '#ffe2a8';
// The pool: a ragged-edged puddle with a skin that swirls, a bright lip where it stands proud of the
// page, and heat showing through in patches.
var MELT_FS = [
  'uniform float uR, uT, uHeat, uA; uniform vec3 uCol, uDeep, uHot; varying vec2 vUv;',
  'float mh( vec2 p ) { return fract( sin( dot( p, vec2( 127.1, 311.7 ) ) ) * 43758.5453 ); }',
  'float mn( vec2 p ) { vec2 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f ); return mix( mix( mh( i ), mh( i + vec2( 1.0, 0.0 ) ), f.x ), mix( mh( i + vec2( 0.0, 1.0 ) ), mh( i + vec2( 1.0, 1.0 ) ), f.x ), f.y ); }',
  'void main() {',
  '  vec2 p = vUv * 2.0 - 1.0; float r = length( p ); vec2 d = p / max( r, 0.0001 );',
  '  float edge = uR * ( 0.5 + 0.22 * mn( d * 1.3 + 3.0 ) + 0.2 * mn( d * 3.1 + 11.0 ) + 0.08 * mn( d * 7.0 + uT * 0.2 ) );',
  '  float in0 = edge - r; if ( in0 < 0.0 ) discard;',
  '  float swirl = mn( p * 4.0 + vec2( uT * 0.22, -uT * 0.17 ) ) * 0.6 + mn( p * 9.0 - uT * 0.3 ) * 0.4;',
  '  vec3 col = mix( uDeep, uCol, smoothstep( 0.0, 0.16, in0 ) * ( 0.75 + 0.25 * swirl ) );',
  '  col = mix( col, uHot, uHeat * smoothstep( 0.5, 0.85, swirl ) * 0.7 );',
  '  float lip = smoothstep( 0.0, 0.018, in0 ) * ( 1.0 - smoothstep( 0.02, 0.07, in0 ) );',
  '  float lit = max( 0.0, dot( d, normalize( vec2( -0.55, 0.8 ) ) ) );',
  '  col += vec3( 1.0 ) * lip * ( 0.12 + 0.6 * lit * lit );',
  '  col *= 1.0 - 0.35 * ( 1.0 - smoothstep( 0.0, 0.035, in0 ) ) * ( 1.0 - lit );',
  '  gl_FragColor = vec4( col, uA * smoothstep( 0.0, 0.008, in0 ) );',
  '  #include <colorspace_fragment>',
  '}'].join('\n');
function meltPool(T, x, size, col, deep) {
  var mat = new T.ShaderMaterial({ transparent: true, depthWrite: false, toneMapped: false,
    uniforms: { uR: { value: 0 }, uT: { value: x.rnd() * 20 }, uHeat: { value: 1 }, uA: { value: 1 }, uCol: { value: new T.Color(col) }, uDeep: { value: new T.Color(deep) }, uHot: { value: new T.Color(MELT_HOT) } },
    vertexShader: 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }', fragmentShader: MELT_FS });
  var m = new T.Mesh(new T.PlaneGeometry(size, size), mat); m.rotation.x = -Math.PI / 2; m.rotation.z = x.rnd() * 6.28; m.position.set(x.c.x, 0.03, x.c.z); m.renderOrder = -1;
  return x.add(m);
}
// The colours a die melts to: its own body where the set gives one, hot metal where it does not. A
// pale die takes on more of the heat, or it would melt to milk, and its number is then drawn dark.
function meltColours(th) {
  var g = th && th.base && th.base.grad;
  if (!g) return { col: '#c8482a', deep: '#4a1408', pale: false };
  var pale = isLight(th), col = mix(g[0], g[1], 0.35);
  return { col: pale ? mix(col, MELT_HOT, 0.45) : col, deep: pale ? mix(MELT_HOT, '#3a0c00', 0.6) : mix(g[1], '#000000', 0.35), pale: pale };
}

function fxMelt(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.2, hd = x.hd, rnd = x.rnd, pc = meltColours(th);
  var dim = dimmer(st, c, 8 * s); x.own.push(dim);
  var pool = meltPool(T, x, 8.4 * s, pc.col, pc.deep), PU = pool.material.uniforms;
  var heat = x.add(flat(T, softTex(T), 7 * s, 7 * s, { add: true, color: MELT_HOT, opacity: 0, y: 0.05, order: 6 })); heat.position.set(c.x, 0.05, c.z);
  var bed = x.add(flat(T, softTex(T), 2.6 * s, 2.6 * s, { color: pc.pale ? MELT_WHITE : 0x1a0600, opacity: 0, y: 0.055, order: 7 }));
  var num = x.add(numeral(T, die, fi, pc.pale ? '#3a0c00' : '#fff6dc', { add: !pc.pale, solid: pc.pale, opacity: 0, y: 0.06, order: 8 })); num.position.set(c.x, 0.06, c.z);
  // The page is the surface of the melt: what has sunk under it is not drawn.
  var plane = new T.Plane(new T.Vector3(0, 1, 0), -0.035); x.clip([plane]);
  var deep = x.p0.y + s * 1.25, lean = new T.Quaternion(), axis = new T.Vector3(rnd() - 0.5, 0, rnd() - 0.5).normalize(), done = {}, lit = false;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt; PU.uT.value += dt;
    dim.material.opacity = 0.4 * smooth(0.1, 0.9, t) * (1 - smooth(4.4, 5.1, t));
    // Down into the pool, held, and back up out of it.
    var sunk = smooth(0.7, 2.0, t) * (1 - smooth(3.0, 4.1, t)), hot = smooth(0.05, 0.7, t) * (1 - smooth(4.0, 4.9, t)), jit = s * 0.03 * smooth(0.1, 0.6, t) * (1 - smooth(0.8, 1.2, t));
    x.g.position.set(x.p0.x + (rnd() - 0.5) * jit, x.p0.y - deep * sunk, x.p0.z + (rnd() - 0.5) * jit);
    x.g.quaternion.copy(x.q0).premultiply(lean.setFromAxisAngle(axis, 0.5 * sunk));
    x.g.scale.set(1 + 0.3 * sunk, 1 - 0.25 * sunk, 1 + 0.3 * sunk); x.g.visible = sunk < 0.995; x.showLoose(sunk < 0.45);
    tintDie(die, MELT_HOT, 0.85 * hot); shadeDie(die, 1 - 0.25 * hot);
    PU.uR.value = out3((t - 0.55) / 1.6) * (1 - smooth(3.0, 4.3, t)) * (1 + 0.025 * Math.sin(t * 5)); PU.uHeat.value = 0.4 + 0.6 * hot; pool.visible = PU.uR.value > 0.01;
    heat.material.opacity = 0.42 * hot * (0.85 + 0.15 * Math.sin(t * 11));
    // The number, afloat: it bobs and drifts a little, and goes under as the die stands back up.
    var afloat = smooth(1.6, 2.05, t) * (1 - smooth(3.2, 3.7, t));
    num.material.opacity = afloat; num.rotation.z = 0.12 * Math.sin(t * 1.7); num.scale.set(2.7 * (1 + 0.06 * Math.sin(t * 3.1)), 2.7 * (1 + 0.06 * Math.sin(t * 2.3 + 1)), 1);
    num.position.set(c.x + 0.12 * s * Math.sin(t * 1.3), 0.06, c.z + 0.1 * s * Math.cos(t * 1.1)); bed.position.set(num.position.x, 0.055, num.position.z); bed.material.opacity = 0.6 * afloat;
    if (t >= 0.6) once('drip', function () {
      st.fx.push(spray(st, { n: hd ? 110 : 46, colors: [MELT_HOT, MELT_WHITE, '#ff3a10'], size: s * 0.07, life: [0.5, 1.2], drag: 0.9, g: 5,
        spawn: function (j, r) { var a = r() * 6.28, v = 0.6 + r() * 2.4; return { p: [c.x + Math.cos(a) * s * 0.7, x.p0.y * (0.4 + r() * 0.8), c.z + Math.sin(a) * s * 0.7], v: [Math.cos(a) * v, 0.8 + r() * 2.2, Math.sin(a) * v], delay: r() * 1.5 }; } }));
      // Heat haze and smoke going up off the die.
      st.fx.push(spray(st, { n: hd ? 40 : 18, colors: ['#5a5560', '#8a8088'], size: s * 0.5, life: [1.2, 2.2], drag: 0.6, g: -1.2, noFloor: true, smoke: true, blend: 'normal', alpha: 0.3,
        spawn: function (j, r) { var a = r() * 6.28, rr = r() * s * 1.4; return { p: [c.x + Math.cos(a) * rr, 0.3 * s, c.z + Math.sin(a) * rr], v: [(r() - 0.5) * 0.5, 0.5 + r() * 0.8, -0.4 - r() * 0.6], delay: r() * 3.2 }; } }));
    });
    if (t >= 2.0) once('gone', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, MELT_HOT, s * 0.6, s * 4.4, 0.9));
      // Bubbles break on the pool while it holds.
      st.fx.push(spray(st, { n: hd ? 36 : 16, colors: [MELT_WHITE, MELT_HOT], size: s * 0.11, life: [0.25, 0.5], drag: 3, g: 0, noFloor: true,
        spawn: function (j, r) { var a = r() * 6.28, rr = (0.4 + r() * 1.6) * s; return { p: [c.x + Math.cos(a) * rr, 0.08, c.z + Math.sin(a) * rr], v: [0, 0.25, 0], delay: r() * 1.1 }; } }));
    });
    if (t >= 4.15 && !lit) { lit = true; glowNumeral(die, fi, MELT_HOT);
      st.fx.push(wave(st, { x: c.x, z: c.z }, MELT_WHITE, s * 0.5, s * 2.6, 0.6)); }
    if (t >= END) { x.end(MELT_HOT); return false; }
    return true;
  } };
}
// A softer melt: the die sags into a small pool and stands straight again.
function fxMeltEcho(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.2, pc = meltColours(th), lit = false;
  var pool = meltPool(T, x, 5 * s, pc.col, pc.deep), PU = pool.material.uniforms;
  var plane = new T.Plane(new T.Vector3(0, 1, 0), -0.035); x.clip([plane]);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt; PU.uT.value += dt;
    var k = smooth(0, 0.7, t) * (1 - smooth(1.2, 1.9, t));
    x.g.position.set(x.p0.x, x.p0.y - x.p0.y * 0.7 * k, x.p0.z); x.g.scale.set(1 + 0.2 * k, 1 - 0.2 * k, 1 + 0.2 * k);
    tintDie(die, MELT_HOT, 0.7 * k); PU.uR.value = k; PU.uHeat.value = 0.4 + 0.6 * k; pool.visible = k > 0.01;
    if (t >= 1.6 && !lit) { lit = true; glowNumeral(die, fi, MELT_HOT); }
    if (t >= END) { x.end(MELT_HOT); return false; }
    return true;
  } };
}
registerCrit('melt', { name: 'Meltdown', kind: 'fail', what: 'The die runs too hot. It glows, slumps and sinks into a spreading pool of its own colour until only the number is left floating on the melt, then stands back up out of it and cools.', fx: fxMelt,
  beat: 2.0, peak: 3.6, echo: fxMeltEcho, echoWhat: 'Each other 1 sags into a small pool of its own and straightens.' });
