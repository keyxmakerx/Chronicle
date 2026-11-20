// ---------- Crit success: Dragon gate ----------
// The page under the die turns to a pond. Two koi rise out of the dark and circle the die, closer and
// faster, until they leap over it together and break into gold spray: the carp that clears the gate
// becomes a dragon. The water closes and the 20 is left lit.
import {
  dimmer, glowNumeral, inout, out3, registerCrit, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, picture } from './kit.js';

var KOI_GOLD = '#ffd66b';
var KOI_POND_FS = [
  'uniform float uT, uOpen, uA; varying vec2 vUv;',
  'float kh( vec2 p ) { return fract( sin( dot( p, vec2( 127.1, 311.7 ) ) ) * 43758.5453 ); }',
  'float kn( vec2 p ) { vec2 i = floor( p ), f = fract( p ); f = f * f * ( 3.0 - 2.0 * f ); return mix( mix( kh( i ), kh( i + vec2( 1.0, 0.0 ) ), f.x ), mix( kh( i + vec2( 0.0, 1.0 ) ), kh( i + vec2( 1.0, 1.0 ) ), f.x ), f.y ); }',
  'void main() {',
  '  vec2 p = vUv * 2.0 - 1.0; float r = length( p );',
  // A ragged shore that laps a little.
  '  float shore = uOpen * ( 0.94 + 0.06 * kn( p * 4.0 + uT * 0.25 ) );',
  '  float a = 1.0 - smoothstep( shore * 0.9, shore, r );',
  '  if ( a <= 0.003 ) discard;',
  // Light on the bottom: two sets of bent bands crossing.
  '  vec2 q = p * 8.0;',
  '  float ca = pow( 0.5 + 0.5 * sin( q.x * 1.3 + sin( q.y + uT ) * 1.6 ) * sin( q.y * 1.1 + sin( q.x * 0.8 - uT * 1.2 ) * 1.6 ), 5.0 );',
  '  vec3 col = mix( vec3( 0.05, 0.38, 0.44 ), vec3( 0.01, 0.12, 0.2 ), smoothstep( 0.0, 1.0, r / max( shore, 0.01 ) ) );',
  '  col += ca * vec3( 0.3, 0.55, 0.5 ) * 0.55;',
  // Rings running out from the die.
  '  col += pow( 0.5 + 0.5 * sin( r * 46.0 - uT * 5.0 ), 10.0 ) * ( 1.0 - smoothstep( 0.05, 0.6, r ) ) * 0.3;',
  // A pale lip where water meets page.
  '  col = mix( col, vec3( 0.75, 0.95, 0.95 ), smoothstep( shore * 0.84, shore * 0.97, r ) * 0.45 );',
  '  gl_FragColor = vec4( col, a * uA );',
  '  #include <colorspace_fragment>',
  '}'].join('\n');
function koiPond(T, x, radius) {
  var mat = new T.ShaderMaterial({ uniforms: { uT: { value: 0 }, uOpen: { value: 0 }, uA: { value: 0.93 } }, transparent: true, depthWrite: false,
    vertexShader: 'varying vec2 vUv; void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4( position, 1.0 ); }', fragmentShader: KOI_POND_FS });
  var m = new T.Mesh(new T.PlaneGeometry(radius * 2, radius * 2), mat); m.rotation.x = -Math.PI / 2; m.position.set(x.c.x, 0.015, x.c.z); m.renderOrder = -1;
  return x.add(m);
}
// A koi seen from above, head to the left: body, tail, fins, and its patches of colour.
var koiTexCache = {};
function koiTex(T, kind) {
  if (koiTexCache[kind]) return koiTexCache[kind];
  koiTexCache[kind] = picture(T, 256, 96, function (g) {
    var body = kind ? '#f6c94a' : '#fbf6ee', patch = kind ? ['#1c1a1c', '#e8862a'] : ['#e8402a', '#f08a2a'], fin = kind ? 'rgba(250,215,120,.75)' : 'rgba(255,250,240,.75)';
    function shape() { g.beginPath(); g.moveTo(8, 48); g.bezierCurveTo(20, 18, 80, 16, 130, 30); g.bezierCurveTo(170, 38, 196, 44, 212, 48); g.bezierCurveTo(196, 52, 170, 58, 130, 66); g.bezierCurveTo(80, 80, 20, 78, 8, 48); g.closePath(); }
    // Fins and tail first, so the body lies over their roots.
    g.fillStyle = fin;
    g.beginPath(); g.moveTo(60, 30); g.quadraticCurveTo(84, 0, 104, 8); g.quadraticCurveTo(92, 24, 84, 32); g.fill();
    g.beginPath(); g.moveTo(60, 66); g.quadraticCurveTo(84, 96, 104, 88); g.quadraticCurveTo(92, 72, 84, 64); g.fill();
    g.beginPath(); g.moveTo(204, 48); g.quadraticCurveTo(232, 18, 254, 12); g.quadraticCurveTo(240, 48, 254, 84); g.quadraticCurveTo(232, 78, 204, 48); g.fill();
    g.strokeStyle = 'rgba(255,255,255,.5)'; g.lineWidth = 1; for (var i = -3; i <= 3; i++) { g.beginPath(); g.moveTo(208, 48); g.lineTo(250, 48 + i * 11); g.stroke(); }
    shape(); var gr = g.createLinearGradient(0, 20, 0, 76); gr.addColorStop(0, body); gr.addColorStop(0.5, '#ffffff'); gr.addColorStop(1, body); g.fillStyle = kind ? body : gr; g.fill();
    g.save(); shape(); g.clip();
    g.fillStyle = patch[0]; g.beginPath(); g.ellipse(48, 42, 26, 17, 0.3, 0, 7); g.ellipse(132, 52, 30, 13, -0.2, 0, 7); g.fill();
    g.fillStyle = patch[1]; g.beginPath(); g.ellipse(92, 50, 18, 14, 0.5, 0, 7); g.ellipse(176, 46, 16, 8, 0, 0, 7); g.fill();
    g.strokeStyle = 'rgba(0,0,0,.25)'; g.lineWidth = 6; shape(); g.stroke();
    g.restore();
    g.fillStyle = '#0a0a0c'; g.beginPath(); g.arc(22, 39, 2.6, 0, 7); g.arc(22, 57, 2.6, 0, 7); g.fill();
  });
  koiTexCache[kind].userData.shared = true; return koiTexCache[kind];
}
// A fish is a strip that follows a path: bend(place) asks where each rib of it lies.
function koiFish(T, x, kind, len, wide) {
  var SEG = 14, geo = new T.PlaneGeometry(1, 1, SEG, 1), pos = geo.attributes.position;
  var m = new T.Mesh(geo, new T.MeshBasicMaterial({ map: koiTex(T, kind), transparent: true, depthWrite: false, side: T.DoubleSide, toneMapped: false })); m.frustumCulled = false; m.renderOrder = 4; x.add(m);
  var at = { x: 0, z: 0, nx: 0, nz: 0, y: 0 };
  return { mesh: m, bend: function (place) {
    for (var j = 0; j <= SEG; j++) {
      place(j / SEG * len, at);
      // The top row of the plane is one flank and the bottom row the other.
      pos.setXYZ(j, at.x + at.nx * wide / 2, at.y, at.z + at.nz * wide / 2); pos.setXYZ(j + SEG + 1, at.x - at.nx * wide / 2, at.y, at.z - at.nz * wide / 2);
    }
    pos.needsUpdate = true;
  } };
}
// Where a rib lies for a fish swimming a circle: `back` is how far behind the nose, along the path.
function koiCircle(c, R, ang, dir, y, t, ph) {
  return function (back, at) {
    var a = ang - dir * back / R, wig = Math.sin(t * 9 - back * 4.2 + ph) * 0.11 * back, r = R + wig;
    at.x = c.x + Math.cos(a) * r; at.z = c.z + Math.sin(a) * r; at.nx = Math.cos(a) * dir; at.nz = Math.sin(a) * dir; at.y = y;
  };
}

function fxKoi(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.6, hd = x.hd;
  var dim = dimmer(st, c, 8 * s); x.own.push(dim);
  var pond = koiPond(T, x, 4.2 * s), U = pond.material.uniforms;
  var fish = [koiFish(T, x, 0, 2.3 * s, 0.86 * s), koiFish(T, x, 1, 2.1 * s, 0.8 * s)], ang = x.rnd() * 6.28;
  var light = new T.PointLight(KOI_GOLD, 0, 9 * s, 1.4); light.position.copy(c).setY(1.8 * s); x.add(light);
  var done = {}, lit = false, top = new T.Vector3(0, 1, 0), spin = new T.Quaternion(), LEAP = 3.05, ripple = 0;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.35 * smooth(0, 0.6, t) * (1 - smooth(4.6, 5.5, t));
    U.uT.value = t; U.uOpen.value = out3(t / 0.9) * (1 - inout((t - 4.4) / 1.0));
    // Closer and faster, until the leap.
    var close = smooth(2.1, LEAP, t), R = s * (2.1 - 1.2 * close), speed = 2.0 + 5.5 * close * close;
    ang += speed * dt;
    var leap = Math.sin(Math.PI * Math.max(0, Math.min(1, (t - 2.75) / 0.55))), show = smooth(0.5, 1.1, t) * (t < LEAP + 0.25 ? 1 : 0);
    fish.forEach(function (f, i) {
      f.mesh.material.opacity = show; f.mesh.visible = show > 0.01;
      f.bend(koiCircle(c, R, ang + i * Math.PI, 1, 0.06 + leap * 1.6 * s, t, i * 2));
    });
    // A ring on the water behind each nose now and then.
    ripple -= dt;
    if (ripple <= 0 && t > 0.9 && t < LEAP) { ripple = 0.34; fish.forEach(function (f, i) { var a = ang + i * Math.PI; st.fx.push(wave(st, { x: c.x + Math.cos(a) * R, z: c.z + Math.sin(a) * R }, '#bfeee8', s * 0.2, s * 1.0, 0.7)); }); }
    // The die lifts and turns as they close in, and comes down gold.
    var up = smooth(2.3, LEAP, t) * (1 - smooth(3.5, 4.3, t)), turn = inout((t - 2.3) / 1.8) * 6.2832;
    x.g.position.set(x.p0.x, x.p0.y + up * 0.6 * s, x.p0.z); x.g.quaternion.copy(x.q0).premultiply(spin.setFromAxisAngle(top, turn));
    var gold = smooth(LEAP, LEAP + 0.2, t) * (1 - smooth(4.2, 5.2, t)); tintDie(die, KOI_GOLD, 0.85 * gold); light.intensity = 30 * gold * Math.exp(-(Math.max(0, t - LEAP)) * 1.2);
    if (t >= LEAP + 0.22) once('burst', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, KOI_GOLD, s * 0.6, s * 5.5, 1.1)); st.fx.push(wave(st, { x: c.x, z: c.z }, '#ffffff', s * 0.4, s * 3.2, 0.7));
      // The dragon: a rope of gold sparks that winds outward and up.
      st.fx.push(spray(st, { n: hd ? 260 : 110, colors: [KOI_GOLD, '#fff3c4', '#ffb84a', '#bfeee8'], size: s * 0.08, life: [0.9, 2.0], drag: 0.8, g: -0.3, noFloor: true, swirl: { x: c.x, z: c.z, k: 2.6 },
        spawn: function (j, r) { var a = r() * 6.28, v = 1.5 + r() * 4.5; return { p: [c.x + Math.cos(a) * 0.6 * s, x.p0.y + s, c.z + Math.sin(a) * 0.6 * s], v: [Math.cos(a) * v, 0.6 + r() * 1.6, Math.sin(a) * v], delay: r() * 0.35 }; } }));
    });
    if (t >= LEAP + 0.3 && !lit) { lit = true; glowNumeral(die, fi, KOI_GOLD); }
    if (t >= END) { x.end(KOI_GOLD); return false; }
    return true;
  } };
}
// A small pool opens under the die and one koi goes once round it.
function fxKoiEcho(st, die, fi, th, e) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.1, lit = false;
  var pond = koiPond(T, x, 2.2 * s), U = pond.material.uniforms, f = koiFish(T, x, ((e && e.order) || 0) % 2, 1.5 * s, 0.56 * s), a0 = x.rnd() * 6.28;
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    U.uT.value = t; U.uOpen.value = out3(t / 0.5) * (1 - inout((t - 1.5) / 0.55));
    f.mesh.material.opacity = smooth(0.2, 0.5, t) * (1 - smooth(1.3, 1.6, t));
    f.bend(koiCircle(c, 1.25 * s, a0 + t * 4.4, 1, 0.06, t, 0));
    tintDie(die, KOI_GOLD, 0.5 * smooth(0.6, 0.9, t) * (1 - smooth(1.4, 2.0, t)));
    if (t >= 0.9 && !lit) { lit = true; glowNumeral(die, fi, KOI_GOLD); st.fx.push(wave(st, { x: c.x, z: c.z }, KOI_GOLD, s * 0.4, s * 2.2, 0.6)); }
    if (t >= END) { x.end(KOI_GOLD); return false; }
    return true;
  } };
}
registerCrit('koi', { name: 'Dragon gate', kind: 'succ', what: 'The page under the die turns to a pond. Two koi circle it, closer and faster, then leap over it together and break into gold spray, and the 20 comes down lit.', fx: fxKoi,
  beat: 3.05, peak: 3.9, echo: fxKoiEcho, echoWhat: 'A small pool opens under each other 20 and one koi goes once round it.' });
