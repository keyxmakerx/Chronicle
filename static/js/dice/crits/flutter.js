// ---------- Crit success: Flutter ----------
// The die trembles and a cloud of butterflies bursts out of it, wheeling over the page on beating
// wings. Most fly off the edges; a handful come back and settle round the 20, opening and closing
// their wings in the light, before they scatter into sparks.
import {
  dimmer, glowNumeral, out3, registerCrit, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, picture } from './kit.js';

var FLUTTER_WINGS = ['#3a8cff', '#ff8a1a', '#fff6e8', '#ff7ab8', '#b8f04a', '#b98cff'];
var flutterWing = null;
// One side of a butterfly, drawn pale so each can be dyed its own colour: a forewing, a hindwing,
// dark veins and borders with pale spots, and half the body along the hinge.
function flutterWingTex(T) {
  if (flutterWing) return flutterWing;
  flutterWing = picture(T, 128, 128, function (g) {
    function wing(pts) { g.beginPath(); g.moveTo(pts[0], pts[1]); g.bezierCurveTo(pts[2], pts[3], pts[4], pts[5], pts[6], pts[7]); g.bezierCurveTo(pts[8], pts[9], pts[10], pts[11], pts[0], pts[1]); g.closePath(); }
    [[4, 60, 30, -6, 120, -4, 122, 34, 118, 66, 50, 70], [4, 66, 60, 62, 104, 78, 92, 104, 70, 128, 20, 110]].forEach(function (p) {
      wing(p); var gr = g.createLinearGradient(0, 0, 128, 0); gr.addColorStop(0, '#d8d8d8'); gr.addColorStop(0.5, '#ffffff'); gr.addColorStop(1, '#f0f0f0'); g.fillStyle = gr; g.fill();
      g.save(); g.clip(); g.strokeStyle = '#141018'; g.lineWidth = 13; wing(p); g.stroke();
      g.lineWidth = 1.6; for (var i = 0; i < 7; i++) { g.beginPath(); g.moveTo(4, 62); g.lineTo(128, 6 + i * 20); g.stroke(); }
      g.fillStyle = '#ffffff'; for (var j = 0; j < 9; j++) { var a = j / 9 * 3.2 - 1.3; g.beginPath(); g.arc(64 + Math.cos(a) * 54, 62 + Math.sin(a) * 52, 2.6, 0, 7); g.fill(); }
      g.restore();
    });
    g.fillStyle = '#141018'; g.beginPath(); g.ellipse(3, 62, 5, 30, 0, 0, 7); g.fill();
  });
  flutterWing.userData.shared = true; return flutterWing;
}
// A swarm: two instanced wings to a butterfly, hinged along its body. place() sets one butterfly.
function flutterSwarm(x, n, cols) {
  var T = x.T, mk = function (side) {
    var geo = new T.PlaneGeometry(1, 1); geo.rotateX(-Math.PI / 2); geo.translate(0.5, 0, 0);
    // The left wing is the right one mirrored through the body.
    if (side < 0) { geo.scale(-1, 1, 1); geo.index.array.reverse(); }
    var m = new T.InstancedMesh(geo, new T.MeshBasicMaterial({ map: flutterWingTex(T), transparent: true, alphaTest: 0.3, side: T.DoubleSide, toneMapped: false }), n);
    m.frustumCulled = false; m.renderOrder = 7; x.add(m); return m;
  };
  var R = mk(1), L = mk(-1), col = new T.Color(), m4 = new T.Matrix4(), q = new T.Quaternion(), qf = new T.Quaternion(), e = new T.Euler(), sc = new T.Vector3(), ax = new T.Vector3(0, 0, 1);
  for (var i = 0; i < n; i++) { col.set(cols[i % cols.length]); R.setColorAt(i, col); L.setColorAt(i, col); }
  return { place: function (i, p, heading, flap, size) {
    e.set(0, -heading + Math.PI / 2, 0); q.setFromEuler(e); sc.set(size, size, size);
    m4.compose(p, q.clone().multiply(qf.setFromAxisAngle(ax, flap)), sc); R.setMatrixAt(i, m4);
    m4.compose(p, q.clone().multiply(qf.setFromAxisAngle(ax, -flap)), sc); L.setMatrixAt(i, m4);
  }, commit: function () { R.instanceMatrix.needsUpdate = true; L.instanceMatrix.needsUpdate = true; } };
}

function fxFlutter(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.6, hd = x.hd, rnd = x.rnd;
  var dim = dimmer(st, c, 8 * s); x.own.push(dim);
  var N = hd ? 34 : 16, HOME = hd ? 6 : 4, cols = [th.burn].concat(FLUTTER_WINGS), swarm = flutterSwarm(x, N, cols), p = new T.Vector3();
  var flies = []; for (var i = 0; i < N; i++) flies.push({ at: 0.55 + rnd() * 0.7, a: rnd() * 6.28, w: (0.7 + rnd() * 0.9) * (rnd() > 0.5 ? 1 : -1), R: (1.6 + rnd() * 2.6) * s, f: 17 + rnd() * 9, ph: rnd() * 6, y: 0.9 + rnd() * 0.9, size: (0.34 + rnd() * 0.2) * s, home: i < HOME ? i / HOME * 6.2832 + 0.4 : null });
  var done = {}, lit = false, top = x.p0.y + s * 0.75;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.3 * smooth(0, 0.6, t) * (1 - smooth(4.6, 5.5, t));
    // Before they break out, the die trembles and glows.
    var pre = smooth(0, 0.5, t) * (1 - smooth(0.5, 0.9, t)), jit = s * 0.035 * pre;
    x.g.position.set(x.p0.x + (rnd() - 0.5) * jit, x.p0.y, x.p0.z + (rnd() - 0.5) * jit);
    tintDie(die, th.burn, 0.9 * pre + 0.5 * smooth(2.9, 3.3, t) * (1 - smooth(4.4, 5.2, t)));
    if (t >= 0.55) once('burst', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, th.burn, s * 0.5, s * 3.6, 0.8));
      st.fx.push(spray(st, { n: hd ? 70 : 30, colors: [th.burn, '#ffffff'], size: s * 0.05, life: [0.5, 1.1], drag: 1.4, g: -0.2, noFloor: true,
        spawn: function (j, r) { var a = r() * 6.28, v = 1 + r() * 3; return { p: [c.x, top, c.z], v: [Math.cos(a) * v, 0.5 + r(), Math.sin(a) * v] }; } }));
    });
    for (var i = 0; i < N; i++) {
      var b = flies[i], k = t - b.at;
      if (k < 0 || t > 5.05) { swarm.place(i, p.set(0, -60, 0), 0, 0, 0.001); continue; }
      // Out of the die on a widening spiral.
      var out = out3(k / 1.5), ang = b.a + b.w * k * 1.5, r = b.R * out + Math.sin(k * 2.3 + b.ph) * 0.3 * s, y = top + (b.y - top) * out + Math.sin(k * 3.1 + b.ph) * 0.15;
      var px = c.x + Math.cos(ang) * r, pz = c.z + Math.sin(ang) * r, head = ang + (b.w > 0 ? 1 : -1) * 1.5708, flap = 0.15 + 1.1 * Math.abs(Math.sin(k * b.f + b.ph)), size = b.size * Math.min(1, k / 0.25);
      if (b.home == null) {
        // Most leave: outward, faster and faster, off the page.
        var go = smooth(2.2 + (i % 5) * 0.12, 3.6 + (i % 5) * 0.12, t); r += go * go * 16 * s; px = c.x + Math.cos(ang) * r; pz = c.z + Math.sin(ang) * r; head = ang + (1 - go) * (b.w > 0 ? 1 : -1) * 1.5708;
      } else {
        // A few come home and settle in a ring round the number, wings opening and closing slowly.
        var land = smooth(2.3 + i * 0.1, 3.3 + i * 0.1, t), hx = c.x + Math.cos(b.home) * 1.15 * s, hz = c.z + Math.sin(b.home) * 1.15 * s;
        px += (hx - px) * land; pz += (hz - pz) * land; y += (0.12 - y) * land; head += (b.home + Math.PI - head) * land;
        flap = flap * (1 - land) + land * (0.25 + 0.55 * (0.5 + 0.5 * Math.sin(t * 3 + b.ph)));
      }
      swarm.place(i, p.set(px, y, pz), head, flap, size);
    }
    swarm.commit();
    if (t >= 3.2 && !lit) { lit = true; glowNumeral(die, fi, th.burn); st.fx.push(wave(st, { x: c.x, z: c.z }, th.burn, s * 0.5, s * 2.6, 0.7)); }
    if (t >= 5.05) once('sparks', function () {
      st.fx.push(spray(st, { n: hd ? 80 : 34, colors: cols, size: s * 0.06, life: [0.4, 0.9], drag: 1.2, g: -0.4, noFloor: true,
        spawn: function (j, r) { var a = flies[j % HOME].home, v = 0.5 + r() * 1.6, d = r() * 6.28; return { p: [c.x + Math.cos(a) * 1.15 * s, 0.2, c.z + Math.sin(a) * 1.15 * s], v: [Math.cos(d) * v, 0.5 + r(), Math.sin(d) * v] }; } }));
    });
    if (t >= END) { x.end(th.burn); return false; }
    return true;
  } };
}
// Three butterflies circle the die and fly on.
function fxFlutterEcho(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.0, lit = false, N = 3, swarm = flutterSwarm(x, N, [th.burn, FLUTTER_WINGS[0], FLUTTER_WINGS[2]]), p = new T.Vector3(), a0 = x.rnd() * 6.28;
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    for (var i = 0; i < N; i++) {
      var k = out3(t / 0.5), go = smooth(1.2, 1.9, t), ang = a0 + i * 2.094 + t * 3.2, r = s * (0.3 + 1.1 * k) + go * go * 14 * s;
      swarm.place(i, p.set(c.x + Math.cos(ang) * r, x.p0.y + 0.6 * s + 0.2 * Math.sin(t * 4 + i), c.z + Math.sin(ang) * r), ang + 1.5708 * (1 - go), 0.15 + 1.1 * Math.abs(Math.sin(t * 21 + i * 2)), 0.36 * s * Math.min(1, t / 0.2));
    }
    swarm.commit();
    tintDie(die, th.burn, 0.5 * smooth(0.3, 0.6, t) * (1 - smooth(1.2, 1.9, t)));
    if (t >= 0.7 && !lit) { lit = true; glowNumeral(die, fi, th.burn); }
    if (t >= END) { x.end(th.burn); return false; }
    return true;
  } };
}
registerCrit('flutter', { name: 'Flutter', kind: 'succ', what: 'A cloud of butterflies bursts out of the die and wheels over the page. Most fly off the edges; a few come back and settle round the 20, wings opening and closing.', fx: fxFlutter,
  beat: 0.55, peak: 3.2, echo: fxFlutterEcho, echoWhat: 'Three butterflies circle each other 20 and fly on.' });
