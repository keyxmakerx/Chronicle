// ---------- Crit fail: Engulfed ----------
// A gelatinous cube heaves in from the edge of the page, one lurch at a time, leaving a wet trail. It
// rolls over the die and takes it in: the die hangs inside the jelly among the bones of earlier
// meals, turning slowly and going dim while bubbles rise. Then the cube moves on, and drops it.
import {
  dimmer, glowNumeral, inout, registerCrit, shadeDie, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, flat, picture, softTex } from './kit.js';

var ENGULF_GREEN = '#7dffa0';
var engulfBits = null;
// What is left of earlier meals: a skull, a bone and a coin.
function engulfBitsTex(T) {
  if (engulfBits) return engulfBits;
  engulfBits = picture(T, 192, 64, function (g) {
    g.fillStyle = '#f2ecd8';
    g.beginPath(); g.ellipse(32, 26, 20, 19, 0, 0, 7); g.fill(); g.fillRect(21, 38, 22, 14);
    g.fillStyle = '#20301c'; g.beginPath(); g.ellipse(24, 26, 6, 7, 0, 0, 7); g.ellipse(40, 26, 6, 7, 0, 0, 7); g.fill(); for (var i = 0; i < 4; i++) g.fillRect(24 + i * 5, 44, 1.6, 8);
    g.strokeStyle = '#f2ecd8'; g.lineWidth = 7; g.lineCap = 'round'; g.beginPath(); g.moveTo(78, 46); g.lineTo(114, 18); g.stroke();
    g.fillStyle = '#f2ecd8'; [[74, 42], [80, 52], [112, 12], [120, 20]].forEach(function (p) { g.beginPath(); g.arc(p[0], p[1], 6, 0, 7); g.fill(); });
    g.fillStyle = '#f0c040'; g.beginPath(); g.arc(160, 32, 18, 0, 7); g.fill(); g.strokeStyle = '#a87a10'; g.lineWidth = 3; g.beginPath(); g.arc(160, 32, 12, 0, 7); g.stroke();
  });
  engulfBits.userData.shared = true; return engulfBits;
}
// The cube: clear jelly with a paler skin, its edges picked out so it reads as a solid from above.
function engulfCube(T, x, size) {
  var grp = new T.Group(), geo = new T.BoxGeometry(size, size, size, 1, 1, 1);
  var skin = new T.MeshPhysicalMaterial({ color: '#58d084', transparent: true, opacity: 0.4, roughness: 0.06, clearcoat: 1, clearcoatRoughness: 0.05, depthWrite: false, emissive: new T.Color('#0c4a22'), emissiveIntensity: 0.5 });
  var deep = new T.MeshBasicMaterial({ color: '#0e5a2c', transparent: true, opacity: 0.38, side: T.BackSide, depthWrite: false, toneMapped: false });
  var a = new T.Mesh(geo, deep), b = new T.Mesh(geo, skin); a.renderOrder = 10; b.renderOrder = 12;
  var edge = new T.LineSegments(new T.EdgesGeometry(geo), new T.LineBasicMaterial({ color: '#d8ffe4', transparent: true, opacity: 0.75, depthWrite: false, toneMapped: false })); edge.renderOrder = 13;
  // The sheen on its top: a soft square of light off to one corner.
  var sheen = flat(T, softTex(T), size * 0.5, size * 0.3, { add: true, color: '#e8fff0', opacity: 0.22, y: size / 2 + 0.01, order: 14 }); sheen.position.x = -size * 0.16; sheen.position.z = -size * 0.2;
  grp.add(a, b, edge, sheen);
  var bits = [0, 1, 2].map(function (i) {
    var tex = engulfBitsTex(T).clone(); tex.needsUpdate = true; tex.repeat.set(1 / 3, 1); tex.offset.set(i / 3, 0);
    var m = flat(T, tex, size * 0.3, size * 0.3, { opacity: 0.85, y: size * (-0.2 + i * 0.16), order: 11 }); m.position.x = size * [-0.3, 0.28, 0.1][i]; m.position.z = size * [0.26, 0.2, -0.32][i]; grp.add(m); return m;
  });
  grp.position.y = size / 2 + 0.01;
  return { grp: x.add(grp), skin: skin, deep: deep, edge: edge, sheen: sheen, bits: bits, size: size };
}

function fxEngulf(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.6, hd = x.hd, rnd = x.rnd;
  var dim = dimmer(st, c, 7 * s); x.own.push(dim);
  // It comes from the far side of the page and leaves by the near one.
  var a = Math.atan2(-c.z, -c.x) + (rnd() - 0.5) * 0.5, dx = Math.cos(a), dz = Math.sin(a), FAR = 13 * s, size = 2.9 * s, cube = engulfCube(T, x, size);
  var trail = flat(T, softTex(T), 1, size * 1.05, { color: '#3fae68', opacity: 0, y: 0.012, order: -1 }); trail.rotation.z = -a; x.add(trail);
  var puddle = flat(T, softTex(T), size * 1.5, size * 1.5, { color: '#3fae68', opacity: 0, y: 0.014, order: -1 }); puddle.position.set(c.x, 0.014, c.z); x.add(puddle);
  var done = {}, lit = false, axis = new T.Vector3(0.4, 1, 0.3).normalize(), spin = new T.Quaternion(), IN = 1.9, OUT = 3.5;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  // The way it travels: three lurches in, a pause over the die, then on.
  function along(tt) {
    if (tt < IN) { var u = tt / IN * 3, n = Math.floor(u); return -FAR * (1 - (n + inout(Math.min(1, (u - n) * 1.35))) / 3); }
    if (tt < OUT) return 0;
    var v = (tt - OUT) / 1.7; return FAR * 1.25 * v * v;
  }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.32 * smooth(0, 0.8, t) * (1 - smooth(4.6, 5.5, t));
    var d = along(t), vel = (along(t + 0.02) - d) / 0.02, sq = Math.min(0.2, Math.abs(vel) * 0.012), wob = 0.035 * Math.sin(t * 9);
    cube.grp.position.set(c.x + dx * d, cube.size / 2 * (1 - sq + wob) + 0.01, c.z + dz * d);
    cube.grp.rotation.y = -a; cube.grp.scale.set(1 + sq * 1.2, 1 - sq + wob, 1 - sq * 0.4 - wob * 0.5);
    cube.bits.forEach(function (b, i) { b.rotation.z = t * (0.4 + i * 0.3) + i; b.position.y = cube.size * (-0.2 + i * 0.16) + 0.06 * Math.sin(t * 2 + i * 2); });
    var gone = smooth(4.6, 5.4, t); cube.skin.opacity = 0.4 * (1 - gone); cube.deep.opacity = 0.38 * (1 - gone); cube.edge.material.opacity = 0.75 * (1 - gone); cube.sheen.material.opacity = 0.22 * (1 - gone);
    cube.bits.forEach(function (b) { b.material.opacity = 0.85 * (1 - gone); });
    // The trail it leaves, from where it came in to where it is.
    var len = Math.max(0.01, FAR + Math.min(d, 0) + 0.5 * size);
    trail.scale.set(len, 1, 1); trail.position.set(c.x + dx * (d - len / 2 + (d > 0 ? -d : 0)), 0.012, c.z + dz * (d - len / 2 + (d > 0 ? -d : 0)));
    trail.material.opacity = 0.22 * smooth(0.2, 0.9, t) * (1 - smooth(4.0, 5.3, t)); puddle.material.opacity = 0.5 * smooth(IN, IN + 0.4, t) * (1 - smooth(4.3, 5.4, t));
    // Inside: the die floats up, turns, and dims; let go, it drops back the way it lay.
    var held = smooth(IN - 0.15, IN + 0.25, t) * (1 - smooth(OUT + 0.15, OUT + 0.6, t));
    x.g.position.set(x.p0.x, x.p0.y + held * 0.55 * s + held * 0.08 * s * Math.sin(t * 2.4), x.p0.z);
    x.g.quaternion.copy(x.q0).premultiply(spin.setFromAxisAngle(axis, held * Math.sin((t - IN) * 0.9) * 0.9));
    shadeDie(die, 1 - 0.5 * held); tintDie(die, ENGULF_GREEN, 0.45 * held + 0.35 * smooth(OUT + 0.5, OUT + 0.7, t) * (1 - smooth(4.4, 5.3, t)));
    if (t >= IN - 0.1) once('gulp', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, ENGULF_GREEN, s * 0.8, s * 3.6, 0.8));
      st.fx.push(spray(st, { n: hd ? 90 : 40, colors: ['#eafff0', ENGULF_GREEN, '#b8ffd0'], size: s * 0.07, life: [0.6, 1.3], drag: 1.6, g: -0.9, noFloor: true,
        spawn: function (j, r) { var b = r() * 6.28, rr = r() * s * 0.9; return { p: [c.x + Math.cos(b) * rr, x.p0.y + r() * s, c.z + Math.sin(b) * rr], v: [(r() - 0.5) * 0.4, 0.3 + r() * 0.8, (r() - 0.5) * 0.4], delay: r() * 1.5 }; } }));
    });
    if (t >= OUT + 0.6 && !lit) { lit = true; glowNumeral(die, fi, ENGULF_GREEN);
      st.fx.push(spray(st, { n: hd ? 40 : 18, colors: ['#8fe8a8', '#d8ffe4'], size: s * 0.07, life: [0.4, 0.9], drag: 1, g: 4, blend: 'normal',
        spawn: function (j, r) { var b = r() * 6.28, v = 1 + r() * 2; return { p: [c.x, x.p0.y, c.z], v: [Math.cos(b) * v, 1 + r() * 1.5, Math.sin(b) * v] }; } })); }
    if (t >= END) { x.end(ENGULF_GREEN); return false; }
    return true;
  } };
}
// A small ooze swells over the die and sinks away again.
function fxEngulfEcho(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.0, lit = false;
  var blob = new T.Mesh(new T.SphereGeometry(1.25 * s, 20, 12), new T.MeshPhysicalMaterial({ color: '#58d084', transparent: true, opacity: 0.45, roughness: 0.06, clearcoat: 1, depthWrite: false, emissive: new T.Color('#0c4a22'), emissiveIntensity: 0.5 }));
  blob.position.set(c.x, x.p0.y * 0.8, c.z); blob.renderOrder = 12; x.add(blob);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var k = smooth(0, 0.5, t) * (1 - smooth(1.3, 1.9, t)), wob = 0.06 * Math.sin(t * 9);
    blob.scale.set(k * (1 + wob), k * (0.8 - wob), k * (1 - wob)); blob.visible = k > 0.01;
    shadeDie(die, 1 - 0.4 * k); tintDie(die, ENGULF_GREEN, 0.4 * k);
    if (t >= 1.5 && !lit) { lit = true; glowNumeral(die, fi, ENGULF_GREEN); }
    if (t >= END) { x.end(ENGULF_GREEN); return false; }
    return true;
  } };
}
registerCrit('engulf', { name: 'Engulfed', kind: 'fail', what: 'A gelatinous cube heaves in from the edge of the page and rolls over the die. The die hangs inside it among old bones, turning and going dim, until the cube moves on and drops it.', fx: fxEngulf,
  beat: 1.8, peak: 3.5, echo: fxEngulfEcho, echoWhat: 'A small ooze swells over each other 1 and sinks away.' });
