// ---------- Crit success: Bard's encore ----------
// A line of music unrolls from the die in a widening spiral: five staff lines, with a note landing on
// them at every beat while the die keeps time. On the last chord every note flies back into the die
// at once, and the staff rolls up behind them.
import {
  dimmer, glowNumeral, in3, out3, registerCrit, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, flat, picture } from './kit.js';

var ENCORE_GOLD = '#ffd66b', ENCORE_INK = '#fff6dc';
// The notes, drawn white so each can be dyed: a quaver, a crotchet, a beamed pair, a minim.
var encoreNotes = null;
function encoreTex(T) {
  if (encoreNotes) return encoreNotes;
  encoreNotes = picture(T, 256, 96, function (g) {
    g.fillStyle = g.strokeStyle = '#ffffff'; g.lineCap = 'round';
    function head(x, y, open) { g.save(); g.translate(x, y); g.rotate(-0.38); g.beginPath(); g.ellipse(0, 0, 11, 7.5, 0, 0, 7); if (open) { g.lineWidth = 3.4; g.stroke(); } else g.fill(); g.restore(); }
    function stem(x, y) { g.lineWidth = 3; g.beginPath(); g.moveTo(x + 9.5, y - 3); g.lineTo(x + 9.5, y - 56); g.stroke(); }
    head(22, 76); stem(22, 76); g.lineWidth = 3; g.beginPath(); g.moveTo(31.5, 20); g.bezierCurveTo(36, 36, 56, 36, 48, 58); g.stroke(); g.beginPath(); g.moveTo(31.5, 22); g.bezierCurveTo(40, 30, 50, 34, 49, 46); g.lineTo(31.5, 34); g.fill();
    head(86, 76); stem(86, 76);
    head(140, 80); stem(140, 80); head(176, 70); stem(176, 70); g.beginPath(); g.moveTo(148, 22); g.lineTo(187, 12); g.lineTo(187, 22); g.lineTo(148, 32); g.closePath(); g.fill();
    head(224, 76, true); stem(224, 76);
  });
  encoreNotes.userData.shared = true; return encoreNotes;
}
function encoreNote(T, x, kind, size, color) {
  var tex = encoreTex(T).clone(); tex.needsUpdate = true; tex.repeat.set(kind === 2 ? 0.3 : 0.2, 1); tex.offset.set([0.02, 0.26, 0.48, 0.79][kind], 0);
  var m = flat(T, tex, size * (kind === 2 ? 0.95 : 0.64), size, { color: color, y: 0.09, order: 9 }); return x.add(m);
}
// Five lines along a spiral from angle a0, as one mesh whose first `shown` rungs are drawn.
function encoreStaff(T, x, a0, turns, r0, grow, gap, dir) {
  var N = Math.round(60 * turns) + 8, pos = [], idx = [], n = 0, wide = 0.022 * x.s;
  function at(u, off) { var a = a0 + dir * u * turns * 6.2832, r = r0 + grow * u * turns + off; return [x.c.x + Math.cos(a) * r, x.c.z + Math.sin(a) * r, Math.cos(a), Math.sin(a)]; }
  for (var k = 0; k <= N; k++) for (var l = 0; l < 5; l++) { var p = at(k / N, (l - 2) * gap); pos.push(p[0] + p[2] * wide, 0, p[1] + p[3] * wide, p[0] - p[2] * wide, 0, p[1] - p[3] * wide); }
  for (k = 0; k < N; k++) for (l = 0; l < 5; l++) { var a = (k * 5 + l) * 2, b = a + 10; idx.push(a, a + 1, b, a + 1, b + 1, b); }
  var geo = new T.BufferGeometry(); geo.setAttribute('position', new T.Float32BufferAttribute(pos, 3)); geo.setIndex(idx);
  var m = new T.Mesh(geo, new T.MeshBasicMaterial({ color: ENCORE_INK, transparent: true, opacity: 0.95, depthWrite: false, side: T.DoubleSide, toneMapped: false }));
  m.position.y = 0.07; m.renderOrder = 8; m.frustumCulled = false; x.add(m);
  return { at: at, N: N, show: function (from, to) { var f = Math.round(from * N), tt = Math.round(to * N); geo.setDrawRange(f * 30, Math.max(0, tt - f) * 30); } };
}

function fxEncore(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 4.9, hd = x.hd, rnd = x.rnd, BEAT = 0.3, CHORD = 2.85;
  var dim = dimmer(st, c, 8 * s); x.own.push(dim);
  var gap = 0.13 * s, staff = encoreStaff(T, x, rnd() * 6.28, 1.45, 1.45 * s, 1.5 * s, gap, 1);
  var cols = [ENCORE_GOLD, th.burn, '#fff6dc'], NN = 8, notes = [];
  for (var i = 0; i < NN; i++) {
    var u = 0.1 + i / NN * 0.86, line = Math.floor(rnd() * 5) - 2, p = staff.at(u, line * gap + 0.34 * s), kind = [1, 0, 2, 1, 3, 0, 2, 1][i];
    notes.push({ m: encoreNote(T, x, kind, 1.45 * s, cols[i % 3]), u: u, x: p[0], z: p[1], at: 0.35 + i * BEAT, done: false });
  }
  var lit = false, burst = false, top = new T.Vector3();
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.62 * smooth(0, 0.5, t) * (1 - smooth(3.9, 4.8, t));
    // The staff unrolls ahead of the notes, and rolls up from its far end after the chord.
    staff.show(0, out3(t / 2.6) * (1 - smooth(CHORD + 0.1, CHORD + 1.2, t)));
    var pulse = Math.pow(Math.abs(Math.sin(Math.PI * t / BEAT)), 0.6);
    notes.forEach(function (n, i) {
      var k = out3((t - n.at) / 0.2), fly = in3((t - CHORD - i * 0.03) / 0.36), bob = 0.05 * s * Math.sin(Math.PI * (t - n.at) / BEAT);
      n.m.visible = t >= n.at && fly < 1; if (!n.m.visible) return;
      n.m.position.set(n.x + (c.x - n.x) * fly, 0.09 + 0.6 * s * Math.sin(Math.PI * fly), n.z + (c.z - n.z) * fly + bob * (1 - fly));
      n.m.scale.setScalar(k * (1 - 0.6 * fly));
      if (!n.done) { n.done = true; st.fx.push(spray(st, { n: hd ? 12 : 6, colors: [ENCORE_GOLD, '#ffffff'], size: s * 0.05, life: [0.3, 0.6], drag: 1.5, g: -0.3, noFloor: true,
        spawn: function (j, r) { var a = r() * 6.28, v = 0.4 + r() * 1.2; return { p: [n.x, 0.2, n.z], v: [Math.cos(a) * v, 0.4 + r() * 0.6, Math.sin(a) * v] }; } })); }
    });
    // The die keeps time, a small hop to the beat, until the chord lands.
    var keep = smooth(0.3, 0.5, t) * (1 - smooth(CHORD, CHORD + 0.2, t));
    x.g.position.set(x.p0.x, x.p0.y + 0.16 * s * pulse * keep, x.p0.z);
    var ring = smooth(CHORD + 0.3, CHORD + 0.42, t) * (1 - smooth(3.6, 4.6, t)); tintDie(die, ENCORE_GOLD, 0.25 * pulse * keep + 0.9 * ring);
    if (t >= CHORD + 0.36 && !burst) { burst = true; top.copy(c).setY(x.p0.y + 0.6 * s);
      [0, 0.12, 0.24].forEach(function (d, i) { st.fx.push({ t: 0, w: null, update: function (d2) { this.t += d2; if (!this.w && this.t >= d) this.w = wave(st, { x: c.x, z: c.z }, cols[i], s * 0.5, s * (3.4 + i * 1.3), 0.9); return this.w ? this.w.update(d2) : true; } }); });
      st.fx.push(spray(st, { n: hd ? 150 : 60, colors: cols, size: s * 0.07, life: [0.7, 1.5], drag: 1.2, g: -0.3, noFloor: true,
        spawn: function (j, r) { var a = r() * 6.28, v = 1 + r() * 4; return { p: [top.x, top.y, top.z], v: [Math.cos(a) * v, 0.4 + r() * 1.4, Math.sin(a) * v] }; } })); }
    if (t >= CHORD + 0.4 && !lit) { lit = true; glowNumeral(die, fi, ENCORE_GOLD); }
    if (t >= END) { x.end(ENCORE_GOLD); return false; }
    return true;
  } };
}
// One bar: a short arc of staff by the die and three notes that land and fly in.
function fxEncoreEcho(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 1.9, lit = false, gap = 0.1 * s;
  var staff = encoreStaff(T, x, x.rnd() * 6.28, 0.42, 1.35 * s, 0.2 * s, gap, 1), notes = [0, 1, 2].map(function (i) {
    var p = staff.at(0.2 + i * 0.3, (i - 1) * gap + 0.26 * s); return { m: encoreNote(T, x, [1, 0, 2][i], 1.05 * s, [ENCORE_GOLD, th.burn, '#fff6dc'][i]), x: p[0], z: p[1], at: 0.2 + i * 0.2 };
  });
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    staff.show(0, out3(t / 0.6) * (1 - smooth(1.2, 1.7, t)));
    notes.forEach(function (n, i) {
      var k = out3((t - n.at) / 0.18), fly = in3((t - 1.05 - i * 0.04) / 0.3);
      n.m.visible = t >= n.at && fly < 1; n.m.position.set(n.x + (c.x - n.x) * fly, 0.09 + 0.4 * s * Math.sin(Math.PI * fly), n.z + (c.z - n.z) * fly); n.m.scale.setScalar(k * (1 - 0.6 * fly));
    });
    tintDie(die, ENCORE_GOLD, 0.7 * smooth(1.3, 1.4, t) * (1 - smooth(1.5, 1.85, t)));
    if (t >= 1.38 && !lit) { lit = true; glowNumeral(die, fi, ENCORE_GOLD); st.fx.push(wave(st, { x: c.x, z: c.z }, ENCORE_GOLD, s * 0.4, s * 2.2, 0.6)); }
    if (t >= END) { x.end(ENCORE_GOLD); return false; }
    return true;
  } };
}
registerCrit('encore', { name: "Bard's encore", kind: 'succ', what: 'A line of music unrolls from the die in a spiral, a note landing on it at every beat while the die keeps time. On the last chord every note flies back into the die at once.', fx: fxEncore,
  beat: 2.85, peak: 3.5, echo: fxEncoreEcho, echoWhat: 'One bar of three notes plays beside each other 20.' });
