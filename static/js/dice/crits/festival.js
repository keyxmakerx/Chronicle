// ---------- Crit success: Festival ----------
// The table throws a party. Rockets leave the die for four corners of the page and break into
// chrysanthemums of colour that crackle as they fade, paper confetti comes down over everything, and
// the last shell bursts gold right over the die, which jumps for it.
import {
  dimmer, glow, glowNumeral, registerCrit, smooth, spray, starTex, tintDie, wave
} from '../engine.js';
import { critScene } from './kit.js';

var FEST_GOLD = '#ffd66b';
var FEST_SHELLS = [['#ff4f6a', '#ffd0d8'], ['#4fd0ff', '#d8f6ff'], ['#7cff6b', '#e4ffd8'], ['#c08cff', '#f2e4ff'], ['#ff9a3a', '#ffe2b8']];
var FEST_PAPER = ['#ff4f6a', '#ffd23f', '#4fd0ff', '#7cff6b', '#c08cff', '#ffffff', '#ff9a3a'];

// A shell's flight (a line of sparks, each lit as the rocket passes) and its break. The break is a
// ball of stars seen from one side: each star flies out from the middle with a tail of sparks behind it
// on the same path, so the burst reads as petals and not as a ring, and the whole flower sinks down the
// page as it burns out.
function festShell(st, x, from, to, at, cols, big) {
  var s = x.s, hd = x.hd, rnd = x.rnd, fly = 0.42, t0 = at + fly;
  st.fx.push(spray(st, { n: hd ? 26 : 12, colors: ['#fff3c4', '#ffb84a'], size: s * 0.045, life: [0.25, 0.5], drag: 2, g: 0.6, noFloor: true,
    spawn: function (j, r) { var u = r(); return { p: [from.x + (to.x - from.x) * u, 0.5 + u * 0.6, from.z + (to.z - from.z) * u], v: [(r() - 0.5) * 0.3, 0, (r() - 0.5) * 0.3], delay: at + u * fly }; } }));
  var K = (big ? 54 : 30) * (hd ? 1 : 0.5) | 0, TAIL = hd ? 9 : 4, v0 = (big ? 8.6 : 6.4) * s, stars = [];
  for (var i = 0; i < K; i++) {
    // A direction on a ball, flattened onto the page: most stars reach the rim, a few stop short of it.
    var a = (i + rnd()) / K * 6.2832, up = rnd() * 2 - 1, v = v0 * Math.sqrt(1 - up * up * 0.8) * (0.94 + rnd() * 0.12);
    stars.push([Math.cos(a) * v, up * 0.5, Math.sin(a) * v]);
  }
  // A steady pull toward a point far down the page is the flower's droop.
  var droop = { x: to.x, y: 1.1, z: to.z + 600 * s, k: (big ? 0.0034 : 0.0028) }, drag = big ? 1.8 : 2.1, y = big ? 1.5 : 1.1;
  st.fx.push(spray(st, { n: K, colors: ['#ffffff', cols[1]], size: s * (big ? 0.15 : 0.12), life: big ? [1.5, 2.2] : [1.0, 1.5], drag: drag, pull: droop, noFloor: true,
    spawn: function (j) { return { p: [to.x, y, to.z], v: stars[j], delay: t0 }; } }));
  st.fx.push(spray(st, { n: K * TAIL, colors: cols, size: s * (big ? 0.085 : 0.07), life: big ? [0.7, 1.7] : [0.5, 1.2], drag: drag, pull: droop, noFloor: true,
    spawn: function (j) { return { p: [to.x, y, to.z], v: stars[j % K], delay: t0 + (1 + (j / K | 0)) * (hd ? 0.022 : 0.035) }; } }));
  // The crackle: the tips spit as they die.
  st.fx.push(spray(st, { n: hd ? 46 : 18, colors: ['#ffffff', cols[1]], size: s * 0.07, life: [0.12, 0.3], drag: 0, g: 0, noFloor: true,
    spawn: function (j, r) { var a = r() * 6.2832, d = (0.55 + 0.45 * Math.sqrt(r())) * v0 / drag; return { p: [to.x + Math.cos(a) * d, 0.8, to.z + Math.sin(a) * d + (big ? 0.5 : 0.35) * s], v: [0, 0, 0], delay: t0 + 0.6 + r() * 0.6 }; } }));
}

function fxFestival(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.2, hd = x.hd, rnd = x.rnd;
  var dim = dimmer(st, c, 9 * s); x.own.push(dim);
  var a0 = rnd() * 6.28, shells = [0, 1, 2, 3].map(function (i) {
    var a = a0 + i * 1.5708 + (rnd() - 0.5) * 0.5, d = (2.7 + rnd() * 0.9) * s;
    return { to: { x: c.x + Math.cos(a) * d, z: c.z + Math.sin(a) * d }, at: 0.15 + i * 0.3, cols: FEST_SHELLS[(i + Math.floor(a0 * 3)) % FEST_SHELLS.length] };
  });
  shells.forEach(function (sh) { festShell(st, x, c, sh.to, sh.at, sh.cols, false); });
  festShell(st, x, c, { x: c.x, z: c.z }, 1.9, [FEST_GOLD, '#fff3c4', th.burn], true);
  // Flashes: a star of light at each break.
  var flashes = shells.map(function (sh) { var f = glow(st, starTex(T), sh.cols[1], s * 3); f.position.set(sh.to.x, 1.2, sh.to.z); x.add(f); return { f: f, at: sh.at + 0.42 }; });
  var fin = glow(st, starTex(T), '#fff3c4', s * 6); fin.position.set(c.x, 1.6, c.z); x.add(fin); flashes.push({ f: fin, at: 2.32 });
  // Confetti: paper that tumbles as it falls, lies where it lands, and is swept away at the end.
  var N = hd ? 150 : 60, cg = new T.PlaneGeometry(0.16 * s, 0.09 * s), cm = new T.MeshBasicMaterial({ transparent: true, opacity: 1, side: T.DoubleSide, depthWrite: false, toneMapped: false });
  var conf = new T.InstancedMesh(cg, cm, N), bits = [], m4 = new T.Matrix4(), q = new T.Quaternion(), e = new T.Euler(), v3 = new T.Vector3(), one = new T.Vector3(1, 1, 1), col = new T.Color();
  for (var i = 0; i < N; i++) {
    var a = rnd() * 6.28, d = Math.sqrt(rnd()) * 5.5 * s;
    bits.push({ x: c.x + Math.cos(a) * d, z: c.z + Math.sin(a) * d, y0: 3 + rnd() * 4, at: 0.5 + rnd() * 2.2, fall: 1.1 + rnd() * 0.8, rx: rnd() * 6, ry: rnd() * 6, sx: 4 + rnd() * 7, sy: 3 + rnd() * 5, sway: rnd() * 6, lie: rnd() * 6.28 });
    conf.setColorAt(i, col.set(FEST_PAPER[i % FEST_PAPER.length]));
  }
  conf.frustumCulled = false; conf.renderOrder = 6; x.add(conf);
  var waved = false, lit = false;
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.5 * smooth(0, 0.5, t) * (1 - smooth(4.2, 5.1, t));
    flashes.forEach(function (fl, i) {
      var k = t - fl.at; fl.f.material.opacity = k < 0 ? 0 : k < 0.05 ? k / 0.05 : Math.exp(-(k - 0.05) * 7);
      if (k >= 0 && i === 4 && !waved) { waved = true; st.fx.push(wave(st, { x: c.x, z: c.z }, FEST_GOLD, s * 0.4, s * 4.5, 0.6)); }
    });
    for (var i = 0; i < N; i++) {
      var b = bits[i], k = Math.max(0, Math.min(1, (t - b.at) / b.fall)), y = b.y0 * (1 - k) + 0.03, air = 1 - k;
      e.set(b.rx + t * b.sx * air, b.lie + t * 0.6 * air, b.ry + t * b.sy * air); if (k >= 1) e.set(-Math.PI / 2, 0, b.lie);
      q.setFromEuler(e); v3.set(b.x + Math.sin(t * 3 + b.sway) * 0.25 * air, t < b.at ? -50 : y, b.z + Math.cos(t * 2.3 + b.sway) * 0.25 * air);
      m4.compose(v3, q, one); conf.setMatrixAt(i, m4);
    }
    conf.instanceMatrix.needsUpdate = true; cm.opacity = 1 - smooth(4.3, 5.1, t);
    // The die jumps for the last shell and lands lit.
    var hop = Math.max(0, Math.sin(Math.PI * Math.min(1, Math.max(0, (t - 2.25) / 0.55))));
    x.g.position.set(x.p0.x, x.p0.y + hop * 0.9 * s, x.p0.z);
    tintDie(die, FEST_GOLD, 0.8 * smooth(2.3, 2.5, t) * (1 - smooth(3.2, 4.4, t)));
    if (t >= 2.36 && !lit) { lit = true; glowNumeral(die, fi, FEST_GOLD); }
    if (t >= END) { x.end(FEST_GOLD); return false; }
    return true;
  } };
}
// One small shell breaks over the die.
function fxFestivalEcho(st, die, fi, th, e) {
  var x = critScene(st, die, fi), s = x.s, c = x.c, t = 0, END = 1.7, lit = false;
  festShell(st, x, c, { x: c.x, z: c.z }, 0, FEST_SHELLS[((e && e.order) || 0) % FEST_SHELLS.length], false);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    tintDie(die, FEST_GOLD, 0.6 * smooth(0.4, 0.55, t) * (1 - smooth(0.9, 1.6, t)));
    if (t >= 0.45 && !lit) { lit = true; glowNumeral(die, fi, FEST_GOLD); }
    if (t >= END) { x.end(FEST_GOLD); return false; }
    return true;
  } };
}
registerCrit('festival', { name: 'Festival', kind: 'succ', what: 'Rockets leave the die and break into fireworks at four corners of the page, confetti comes down over everything, and the last shell bursts gold right over the die.', fx: fxFestival,
  beat: 2.32, peak: 3.1, echo: fxFestivalEcho, echoWhat: 'A small firework breaks over each other 20.' });
