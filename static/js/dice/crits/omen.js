// ---------- Crit fail: Ill omen ----------
// The same deck, turned against you. Three cards are dealt from the foot of the page: the Moon, Death
// and, last, the Tower. As the Tower turns, lightning comes down the page and strikes the die; the
// card catches and burns away from one edge, and the other two go to ash.
import {
  dimmer, glowNumeral, inout, out3, registerCrit, shadeDie, smooth, spray, tintDie, wave
} from '../engine.js';
import { cardTex, critScene, tarot } from './kit.js';

var OMEN_RED = '#ff3a2a', OMEN_BOLT = '#d8ccff';
// The burning card: fire eats in from one edge along a ragged line, with embers on the line itself.
var OMEN_BURN_FS = [
  'uniform sampler2D map; uniform float uBurn, uA; varying vec2 vUv;',
  'float oh( vec2 p ) { return fract( sin( dot( p, vec2( 127.1, 311.7 ) ) ) * 43758.5453 ); }',
  'float on( vec2 p ) { vec2 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f ); return mix( mix( oh( i ), oh( i + vec2( 1.0, 0.0 ) ), f.x ), mix( oh( i + vec2( 0.0, 1.0 ) ), oh( i + vec2( 1.0, 1.0 ) ), f.x ), f.y ); }',
  'void main() {',
  '  vec4 c = texture2D( map, vUv );',
  '  float f = on( vUv * 6.0 ) * 0.5 + on( vUv * 17.0 ) * 0.2 + vUv.y * 0.55;',
  '  float e = uBurn * 1.45 - f;',
  '  if ( e > 0.1 ) discard;',
  '  vec3 col = mix( c.rgb, vec3( 0.03, 0.02, 0.02 ), smoothstep( -0.16, 0.0, e ) );',
  '  col = mix( col, vec3( 1.0, 0.42, 0.06 ), smoothstep( 0.0, 0.06, e ) );',
  '  col = mix( col, vec3( 1.0, 0.92, 0.6 ), smoothstep( 0.06, 0.1, e ) );',
  '  gl_FragColor = vec4( col, c.a * uA );',
  '  #include <colorspace_fragment>',
  '}'].join('\n');
// A bolt: a ragged ribbon on the page from far up it down to a point, with two short forks.
function omenBolt(T, from, to, rnd, wide) {
  var pos = [], idx = [], n = 0;
  function run(a, b, steps, w, jag) {
    var dx = b.x - a.x, dz = b.z - a.z, L = Math.hypot(dx, dz), nx = -dz / L, nz = dx / L, pts = [];
    for (var i = 0; i <= steps; i++) { var u = i / steps, j = i && i < steps ? (rnd() - 0.5) * jag : 0; pts.push([a.x + dx * u + nx * j, a.z + dz * u + nz * j]); }
    pts.forEach(function (p, i) {
      var ww = w * (1 - 0.5 * i / steps);
      pos.push(p[0] + nx * ww, 0, p[1] + nz * ww, p[0] - nx * ww, 0, p[1] - nz * ww);
      if (i) idx.push(n + i * 2 - 2, n + i * 2 - 1, n + i * 2, n + i * 2 - 1, n + i * 2 + 1, n + i * 2);
    });
    n += pts.length * 2; return pts;
  }
  var main = run(from, to, 12, wide, wide * 9);
  [4, 7].forEach(function (k) { var p = main[k], a = { x: p[0], z: p[1] }, sg = rnd() > 0.5 ? 1 : -1; run(a, { x: a.x + sg * wide * (14 + rnd() * 10), z: a.z + wide * (8 + rnd() * 10) }, 5, wide * 0.5, wide * 5); });
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setIndex(idx);
  return geo;
}

function fxOmen(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.6, hd = x.hd;
  var dim = dimmer(st, c, 8 * s); x.own.push(dim);
  var names = ['moon', 'death', 'tower'], flipAt = [1.05, 1.6, 2.25];
  var spots = [[-2.5, 1.9, -0.3], [2.5, 1.9, 0.3], [0, 2.9, 0]].map(function (p) { return { x: c.x + p[0] * s, z: c.z + p[1] * s, ry: p[2] }; });
  var cards = names.map(function (n, i) { var cd = tarot(T, n, (i === 2 ? 2.7 : 2.3) * s); x.add(cd.root); return cd; });
  // The Tower's face is its own material, so it can burn.
  var burn = new T.ShaderMaterial({ uniforms: { map: { value: cardTex(T, 'tower') }, uBurn: { value: 0 }, uA: { value: 1 } }, transparent: true, depthWrite: false,
    vertexShader: 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }', fragmentShader: OMEN_BURN_FS });
  cards[2].front.material.dispose(); cards[2].front.material = burn;
  var top = { x: c.x + (x.rnd() - 0.5) * 3 * s, z: c.z - 16 * s };
  var boltGeo = omenBolt(T, top, { x: c.x, z: c.z }, x.rnd, 0.07 * s);
  var bolt = new T.Mesh(boltGeo, new T.MeshBasicMaterial({ color: '#ffffff', transparent: true, opacity: 0, depthWrite: false, depthTest: false, blending: T.AdditiveBlending, toneMapped: false, side: T.DoubleSide }));
  var halo = new T.Mesh(omenBolt(T, top, { x: c.x, z: c.z }, x.rnd, 0.3 * s), new T.MeshBasicMaterial({ color: OMEN_BOLT, transparent: true, opacity: 0, depthWrite: false, depthTest: false, blending: T.AdditiveBlending, toneMapped: false, side: T.DoubleSide }));
  bolt.position.y = halo.position.y = 0.5; bolt.renderOrder = halo.renderOrder = 20; x.add(halo); x.add(bolt);
  var light = new T.PointLight(OMEN_BOLT, 0, 12 * s, 1.2); light.position.copy(c).setY(2 * s); x.add(light);
  var done = {}, lit = false, STRIKE = 2.62;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.62 * smooth(0, 0.7, t) * (1 - smooth(4.6, 5.5, t));
    cards.forEach(function (cd, i) {
      var k = out3((t - 0.12 * i) / 0.7), sp = spots[i];
      cd.root.position.set(sp.x, 0, sp.z + (1 - k) * 14 * s);
      cd.root.rotation.y = sp.ry + (1 - k) * (1 - i) * 0.8;
      cd.turn(inout((t - flipAt[i]) / (i === 2 ? 0.55 : 0.4)));
      // The Moon and Death grey and crumble once the Tower has spoken.
      if (i < 2) cd.fade(1 - smooth(3.4 + i * 0.25, 4.2 + i * 0.25, t));
    });
    cards[2].back.visible = t < flipAt[2] + 0.56;
    burn.uniforms.uBurn.value = smooth(2.9, 4.5, t); burn.uniforms.uA.value = 1;
    cards[2].root.children[0].material.opacity = 0.35 * (1 - smooth(3.0, 4.2, t));
    // Lightning: two flickers and a long strike, each a new brightness.
    var sk = t - STRIKE, flash = sk < 0 ? 0 : sk < 0.07 ? 1 : sk < 0.13 ? 0.15 : sk < 0.22 ? 0.9 : Math.max(0, 0.9 - (sk - 0.22) * 2.6);
    bolt.material.opacity = flash; halo.material.opacity = flash * 0.4; light.intensity = 60 * flash;
    if (sk >= 0) once('strike', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, OMEN_BOLT, s * 0.6, s * 5.5, 0.7));
      st.fx.push(spray(st, { n: hd ? 110 : 50, colors: ['#ffffff', OMEN_BOLT, '#9a7cff'], size: s * 0.05, life: [0.3, 0.9], drag: 1.6, g: 2.4,
        spawn: function (j, r) { var a = r() * 6.28, v = 2 + r() * 5; return { p: [c.x, x.p0.y, c.z], v: [Math.cos(a) * v, 0.5 + r() * 2.5, Math.sin(a) * v] }; } }));
    });
    // The die is knocked dark and shudders, then steadies with the 1 burning.
    var hit = sk < 0 ? 0 : Math.exp(-sk * 2.2), jit = s * 0.06 * hit;
    x.g.position.set(x.p0.x + (x.rnd() - 0.5) * jit, x.p0.y, x.p0.z + (x.rnd() - 0.5) * jit);
    shadeDie(die, 1 - 0.6 * smooth(0.3, 1.2, t) * (1 - smooth(4.6, 5.5, t)));
    tintDie(die, OMEN_RED, 0.6 * hit);
    if (sk >= 0.25 && !lit) { lit = true; glowNumeral(die, fi, OMEN_RED); }
    if (t >= 3.0) once('ash', function () { spots.forEach(function (sp, i) {
      st.fx.push(spray(st, { n: hd ? 70 : 30, colors: i === 2 ? ['#ff7a2a', '#ffd08a', '#3a3038'] : ['#2a2630', '#4a4450', '#16131c'], size: s * 0.07, life: [1.0, 2.2], drag: 0.5, g: -0.5, noFloor: true, blend: i === 2 ? 'add' : 'normal', smoke: i !== 2,
        spawn: function (j, r) { return { p: [sp.x + (r() - 0.5) * 1.4 * s, 0.1 + r() * 0.3, sp.z + (r() - 0.5) * 2.4 * s], v: [(r() - 0.5) * 0.5 + 0.4, 0.4 + r() * 0.9, (r() - 0.5) * 0.5], delay: (i === 2 ? 0 : 0.4 + i * 0.25) + r() * 1.3 }; } })); }); });
    if (t >= END) { x.end(OMEN_RED); return false; }
    return true;
  } };
}
// One small card beside the die turns up Death, and goes to ash.
function fxOmenEcho(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.0, cd = tarot(T, 'death', 1.7 * s), ash = false;
  var a = x.rnd() * 6.28, at = { x: c.x + Math.cos(a) * 1.9 * s, z: c.z + Math.sin(a) * 1.9 * s };
  cd.root.rotation.y = (x.rnd() - 0.5) * 0.8; cd.root.position.set(at.x, 0, at.z); x.add(cd.root);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var k = out3(t / 0.4);
    cd.root.scale.setScalar(0.4 + 0.6 * k); cd.turn(inout((t - 0.45) / 0.35)); cd.fade(k * (1 - smooth(1.2, 1.8, t)));
    shadeDie(die, 1 - 0.45 * smooth(0.6, 0.9, t) * (1 - smooth(1.4, 1.9, t)));
    tintDie(die, OMEN_RED, 0.5 * smooth(0.7, 1.0, t) * (1 - smooth(1.3, 1.8, t)));
    if (t >= 0.9 && !ash) { ash = true; glowNumeral(die, fi, OMEN_RED);
      st.fx.push(spray(st, { n: x.hd ? 34 : 14, colors: ['#2a2630', '#4a4450'], size: s * 0.07, life: [0.8, 1.4], drag: 0.6, g: -0.5, noFloor: true, blend: 'normal', smoke: true,
        spawn: function (j, r) { return { p: [at.x + (r() - 0.5) * s, 0.15, at.z + (r() - 0.5) * 1.6 * s], v: [(r() - 0.5) * 0.5, 0.4 + r() * 0.7, (r() - 0.5) * 0.5], delay: 0.3 + r() * 0.5 }; } })); }
    if (t >= END) { x.end(OMEN_RED); return false; }
    return true;
  } };
}
registerCrit('omen', { name: 'Ill omen', kind: 'fail', what: 'Three tarot cards are dealt and turned: the Moon, Death, and last the Tower. Lightning comes down the page and strikes the die, and the Tower burns away from one edge.', fx: fxOmen,
  beat: 2.62, peak: 3.4, echo: fxOmenEcho, echoWhat: 'A single card lands beside each other 1, turns up Death and goes to ash.' });
