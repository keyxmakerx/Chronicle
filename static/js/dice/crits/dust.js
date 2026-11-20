// ---------- Crit fail: Dust ----------
// A dry wind crosses the page and takes the die with it, a grain at a time, from the windward side.
// What is left is a small dune with the number drawn in it. Then the wind turns, the sand streams
// back, and the die is whole again.
import {
  dimmer, glowNumeral, mix, out3, registerCrit, shadeDie, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, flat, numeral, picture } from './kit.js';

var DUST_PALE = '#f1e3bf', DUST_SAND = '#d2b47c', DUST_DARK = '#8a6a3c';
// One gust: a long thin streak, faint at both ends.
var DUST_STREAK = null;
function dustStreak(T) {
  if (DUST_STREAK) return DUST_STREAK;
  DUST_STREAK = picture(T, 256, 16, function (g) { var gr = g.createLinearGradient(0, 0, 256, 0); gr.addColorStop(0, 'rgba(255,255,255,0)'); gr.addColorStop(0.6, 'rgba(255,255,255,.9)'); gr.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = gr; g.beginPath(); g.ellipse(128, 8, 128, 3.2, 0, 0, 6.2832); g.fill(); });
  DUST_STREAK.userData.shared = true; return DUST_STREAK;
}
// The dune: a heap seen from above, lit from one side, with wind ripples across it.
function dustDune(T, light, dark) {
  return picture(T, 256, 256, function (g) {
    var gr = g.createRadialGradient(96, 100, 6, 128, 128, 124); gr.addColorStop(0, light); gr.addColorStop(0.5, mix(light, dark, 0.4)); gr.addColorStop(0.82, dark); gr.addColorStop(1, 'rgba(0,0,0,0)');
    // A teardrop: blunt to windward, drawn out to a tail downwind.
    g.fillStyle = gr; g.beginPath(); g.moveTo(250, 128); g.bezierCurveTo(190, 60, 120, 34, 70, 44); g.bezierCurveTo(10, 60, 10, 196, 70, 212); g.bezierCurveTo(120, 222, 190, 196, 250, 128); g.fill();
    g.globalCompositeOperation = 'source-atop'; g.lineWidth = 2.4;
    for (var i = 0; i < 9; i++) { var y = 50 + i * 20; g.strokeStyle = i % 2 ? 'rgba(255,255,255,.2)' : 'rgba(60,40,10,.2)'; g.beginPath(); g.moveTo(0, y); g.bezierCurveTo(70, y - 16, 150, y + 14, 256, y - 6); g.stroke(); }
  });
}
// The die's own sand: its body colours where the set gives them, plain sand where it does not.
function dustColours(th) {
  var g = th && th.base && th.base.grad;
  return g ? [g[0], DUST_PALE, mix(g[0], g[1], 0.5), DUST_SAND, DUST_DARK] : [DUST_PALE, DUST_SAND, DUST_DARK];
}

function fxDust(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.0, hd = x.hd, rnd = x.rnd, cols = dustColours(th);
  var dim = dimmer(st, c, 9 * s); x.own.push(dim);
  // The wind blows across the page, a little up or down it.
  var wa = (rnd() < 0.5 ? 0 : Math.PI) + (rnd() - 0.5) * 0.5, wx = Math.cos(wa), wz = Math.sin(wa), R = 1.25 * s;
  var plane = new T.Plane(new T.Vector3(wx, 0, wz), 0); x.clip([plane]);
  var dune = x.add(flat(T, dustDune(T, DUST_PALE, DUST_DARK), 3.3 * s, 3.3 * s, { opacity: 0, y: 0.025, order: -1 })); dune.rotation.z = -wa; dune.position.set(c.x + wx * 0.35 * s, 0.025, c.z + wz * 0.35 * s);
  var num = x.add(numeral(T, die, fi, '#2a1c08', { solid: true, opacity: 0, y: 0.04, order: 2 })); num.position.set(c.x + wx * 0.35 * s, 0.04, c.z + wz * 0.35 * s); num.scale.setScalar(1.9);
  var NG = hd ? 12 : 6, gusts = [];
  for (var i = 0; i < NG; i++) { var gm = x.add(flat(T, dustStreak(T), (3 + rnd() * 5) * s, 0.16 * s, { color: DUST_SAND, opacity: 0, y: 0.05 + i * 0.001, order: 7 })); gm.rotation.z = -wa; gusts.push({ m: gm, off: (rnd() - 0.5) * 9 * s, ph: rnd(), sp: 0.55 + rnd() * 0.5 }); }
  // Where the wind has got to: everything upwind of the front is gone.
  function front(tt) { return -R + 2 * R * (smooth(0.5, 1.9, tt) - smooth(3.0, 4.0, tt)); }
  function grains(n, from, dur, back) {
    return spray(st, { n: n, colors: cols, size: s * 0.07, life: back ? [0.5, 0.9] : [0.9, 1.9], drag: back ? 0.5 : 0.25, g: back ? 0 : 1.6, blend: 'normal', noFloor: !!back, pull: back ? { x: c.x, y: x.p0.y, z: c.z, k: 16 } : null,
      spawn: function (j, r) {
        var d = r() * dur, f = front(from + d), side = (r() - 0.5) * 2, half = Math.sqrt(Math.max(0.05, 1 - Math.pow(f / R, 2))) * R, v = 3 + r() * 5;
        if (back) { var far = (3 + r() * 5) * s; return { p: [c.x + wx * far - wz * side * 2 * s, 0.1 + r() * s, c.z + wz * far + wx * side * 2 * s], v: [-wx * 2, 0.3, -wz * 2], delay: d }; }
        return { p: [c.x + wx * f - wz * side * half, r() * 1.7 * s, c.z + wz * f + wx * side * half], v: [wx * v - wz * (r() - 0.5), 0.4 + r() * 1.6, wz * v + wx * (r() - 0.5)], delay: d };
      } });
  }
  var done = {}, lit = false;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.36 * smooth(0, 0.7, t) * (1 - smooth(4.3, 5.0, t));
    var f = front(t), blow = smooth(0.05, 0.5, t) * (1 - smooth(4.0, 4.7, t)), turn = smooth(2.7, 3.0, t), gone = smooth(1.5, 1.9, t) * (1 - smooth(3.0, 3.4, t));
    plane.constant = -(wx * c.x + wz * c.z + f); x.g.visible = f < R * 0.98; x.showLoose(f < -R * 0.25);
    // The die leans from the wind while it can.
    x.g.position.set(x.p0.x + wx * 0.06 * s * blow * (1 - turn * 2), x.p0.y, x.p0.z + wz * 0.06 * s * blow * (1 - turn * 2));
    shadeDie(die, 1 - 0.2 * blow); tintDie(die, DUST_SAND, 0.25 * blow);
    gusts.forEach(function (gu) {
      var u = (t * gu.sp + gu.ph) % 1, along = (u - 0.5) * 22 * s * (1 - 2 * turn);
      gu.m.position.set(c.x + wx * along - wz * gu.off, gu.m.position.y, c.z + wz * along + wx * gu.off); gu.m.material.opacity = 0.3 * blow * Math.sin(u * Math.PI);
    });
    dune.material.opacity = gone; dune.scale.setScalar(0.5 + 0.5 * out3((t - 1.0) / 1.0) - 0.4 * smooth(3.0, 3.5, t)); num.material.opacity = 0.75 * smooth(1.85, 2.2, t) * (1 - smooth(2.9, 3.15, t));
    if (t >= 0.5) once('blow', function () { st.fx.push(grains(hd ? 900 : 360, 0.5, 1.45, false)); });
    if (t >= 1.9) once('gone', function () { st.fx.push(wave(st, { x: c.x, z: c.z }, DUST_PALE, s * 0.6, s * 4, 0.9)); });
    if (t >= 2.85) once('back', function () { st.fx.push(grains(hd ? 600 : 240, 2.85, 1.1, true)); });
    if (t >= 4.05 && !lit) { lit = true; glowNumeral(die, fi, DUST_PALE);
      st.fx.push(wave(st, { x: c.x, z: c.z }, DUST_PALE, s * 0.5, s * 2.4, 0.5)); }
    if (t >= END) { x.end(DUST_PALE); return false; }
    return true;
  } };
}
// A gust takes a bite out of the die and gives it back.
function fxDustEcho(st, die, fi, th) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.0, rnd = x.rnd, cols = dustColours(th), lit = false, sent = false;
  var wa = rnd() * 6.28, wx = Math.cos(wa), wz = Math.sin(wa), R = 1.25 * s, plane = new T.Plane(new T.Vector3(wx, 0, wz), 0); x.clip([plane]);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var f = -R + 1.1 * R * smooth(0.1, 0.8, t) * (1 - smooth(1.0, 1.7, t)); plane.constant = -(wx * c.x + wz * c.z + f);
    if (!sent) { sent = true; st.fx.push(spray(st, { n: x.hd ? 160 : 70, colors: cols, size: s * 0.07, life: [0.6, 1.2], drag: 0.3, g: 1.6, blend: 'normal',
      spawn: function (j, r) { var d = r() * 0.7, side = (r() - 0.5) * 1.6 * s, v = 2 + r() * 4; return { p: [c.x - wx * R * (1 - d) - wz * side, r() * 1.5 * s, c.z - wz * R * (1 - d) + wx * side], v: [wx * v, 0.4 + r(), wz * v], delay: 0.1 + d }; } })); }
    if (t >= 1.6 && !lit) { lit = true; glowNumeral(die, fi, DUST_PALE); }
    if (t >= END) { x.end(DUST_PALE); return false; }
    return true;
  } };
}
registerCrit('dust', { name: 'Dust', kind: 'fail', what: 'A dry wind crosses the page and takes the die with it a grain at a time, until only a small dune is left with the number drawn in it. Then the wind turns and the sand streams back into a die.', fx: fxDust,
  beat: 1.9, peak: 3.5, echo: fxDustEcho, echoWhat: 'A gust takes a bite out of each other 1 and gives it back.' });
