// ---------- Crit success: Prismatic ----------
// A beam of white light crosses the page and strikes the die, and the die splits it like a prism: a
// fan of seven colours opens on the far side, and flecks of colour dance on the page. Then the die
// turns once on the spot and sweeps the whole rainbow round the table like a lighthouse.
import {
  dimmer, glow, glowNumeral, inout, out3, registerCrit, smooth, spray, starTex, tintDie, wave
} from '../engine.js';
import { critScene, flat, picture } from './kit.js';

var PRISM = ['#ff3b3b', '#ff9a2a', '#ffe93a', '#4fe86a', '#3ac8ff', '#5a6cff', '#c05aff'], PRISM_WHITE = '#fff8ea';
var prismBeam = null, prismWedge = null;
// A beam: bright down its middle, soft at its sides, fading in at its tail.
function prismBeamTex(T) {
  if (prismBeam) return prismBeam;
  prismBeam = picture(T, 128, 32, function (g) {
    var v = g.createLinearGradient(0, 0, 0, 32); v.addColorStop(0, 'rgba(255,255,255,0)'); v.addColorStop(0.5, 'rgba(255,255,255,1)'); v.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = v; g.fillRect(0, 0, 128, 32);
    g.globalCompositeOperation = 'destination-in'; var h = g.createLinearGradient(0, 0, 128, 0); h.addColorStop(0, 'rgba(0,0,0,0)'); h.addColorStop(0.25, 'rgba(0,0,0,1)'); h.addColorStop(1, 'rgba(0,0,0,1)'); g.fillStyle = h; g.fillRect(0, 0, 128, 32);
  });
  prismBeam.userData.shared = true; return prismBeam;
}
// One colour of the fan: a wedge from a point, brightest near the die and gone by its far end.
function prismWedgeTex(T) {
  if (prismWedge) return prismWedge;
  prismWedge = picture(T, 256, 64, function (g) {
    var h = g.createLinearGradient(0, 0, 256, 0); h.addColorStop(0, 'rgba(255,255,255,1)'); h.addColorStop(0.5, 'rgba(255,255,255,.5)'); h.addColorStop(1, 'rgba(255,255,255,0)');
    g.fillStyle = h; g.beginPath(); g.moveTo(0, 32); g.lineTo(256, 0); g.lineTo(256, 64); g.closePath(); g.fill();
  });
  prismWedge.userData.shared = true; return prismWedge;
}
// Lay a flat strip from a point along an angle on the page.
function prismAim(m, at, ang, len, wide, y) {
  m.scale.set(len, wide, 1); m.rotation.z = -ang; m.position.set(at.x + Math.cos(ang) * len / 2, y, at.z + Math.sin(ang) * len / 2);
}

function fxPrism(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.0, hd = x.hd;
  var dim = dimmer(st, c, 10 * s); x.own.push(dim);
  // The beam comes from whichever side of the page is farther, so the fan has room on the other.
  var from = c.x > 0 ? 0 : Math.PI, out = from + Math.PI;
  var beam = x.add(flat(T, prismBeamTex(T), 1, 1, { add: true, color: PRISM_WHITE, opacity: 0, order: 9 }));
  var core = x.add(flat(T, prismBeamTex(T), 1, 1, { add: true, color: '#ffffff', opacity: 0, order: 9 }));
  var wedges = PRISM.map(function (h) { return x.add(flat(T, prismWedgeTex(T), 1, 1, { add: true, color: h, opacity: 0, order: 8 })); });
  var star = glow(st, starTex(T), '#ffffff', s * 4); star.position.copy(c).setY(x.p0.y + 0.9 * s); x.add(star);
  var light = new T.PointLight('#ffffff', 0, 8 * s, 1.4); light.position.copy(c).setY(1.8 * s); x.add(light);
  var done = {}, lit = false, top = new T.Vector3(0, 1, 0), spin = new T.Quaternion();
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.66 * smooth(0, 0.6, t) * (1 - smooth(4.2, 4.9, t));
    // The beam runs in to the die; it stays until the sweep is done.
    var reach = out3(t / 0.45), on = 1 - smooth(4.0, 4.5, t), L = 16 * s;
    var tail = { x: c.x + Math.cos(from) * L, z: c.z + Math.sin(from) * L };
    prismAim(beam, tail, out, L * reach, 0.5 * s, 0.4); prismAim(core, tail, out, L * reach, 0.14 * s, 0.42);
    beam.material.opacity = 0.55 * on; core.material.opacity = 0.95 * on;
    // The die turns once, and the fan goes round the page with it.
    var turn = inout((t - 2.2) / 1.6) * 6.2832, open = out3((t - 0.5) / 0.8) * (1 - smooth(4.0, 4.6, t)), far = s * (2 + 6.5 * out3((t - 0.5) / 1.1));
    x.g.quaternion.copy(x.q0).premultiply(spin.setFromAxisAngle(top, -turn));
    wedges.forEach(function (w, i) {
      var a = out + 0.22 + turn + (i - 3) * 0.085 * (0.35 + 0.65 * open);
      prismAim(w, c, a, far, far * 0.105, 0.3 + i * 0.004); w.material.opacity = 0.85 * open;
    });
    var hit = smooth(0.42, 0.55, t) * (1 - smooth(4.0, 4.6, t));
    star.material.opacity = hit * (0.7 + 0.3 * Math.sin(t * 9)); star.scale.setScalar(s * (3.2 + 0.6 * Math.sin(t * 5)));
    light.intensity = 16 * hit; tintDie(die, PRISM_WHITE, 0.55 * hit);
    if (t >= 0.45) once('hit', function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, '#ffffff', s * 0.5, s * 3, 0.6));
      // Flecks of colour on the page inside the fan, twinkling for as long as it is open.
      st.fx.push(spray(st, { n: hd ? 170 : 70, colors: PRISM, size: s * 0.07, life: [0.25, 0.6], drag: 0, g: 0, noFloor: true,
        spawn: function (j, r) { var a = r() * 6.2832, d = (1.2 + r() * 5.5) * s; return { p: [c.x + Math.cos(a) * d, 0.05, c.z + Math.sin(a) * d], v: [0, 0, 0], delay: 0.2 + r() * 3.3 }; } }));
    });
    if (t >= 2.2) once('ring', function () { PRISM.forEach(function (h, i) { st.fx.push({ t: 0, w: null, update: function (d2) { this.t += d2; if (!this.w && this.t >= i * 0.07) this.w = wave(st, { x: c.x, z: c.z }, h, s * 0.6, s * (4 + i * 0.5), 1.0); return this.w ? this.w.update(d2) : true; } }); }); });
    if (t >= 3.8 && !lit) { lit = true; glowNumeral(die, fi, PRISM_WHITE); }
    if (t >= END) { x.end(PRISM_WHITE); return false; }
    return true;
  } };
}
// A small fan opens from the die, away from the die that cast the first.
function fxPrismEcho(st, die, fi, th, e) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 1.8, lit = false;
  var out = e && e.from ? Math.atan2(c.z - e.from.group.position.z, c.x - e.from.group.position.x) : x.rnd() * 6.28;
  var wedges = PRISM.map(function (h) { return x.add(flat(T, prismWedgeTex(T), 1, 1, { add: true, color: h, opacity: 0, order: 8 })); });
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var open = out3(t / 0.5) * (1 - smooth(1.1, 1.7, t)), far = s * (1.5 + 3 * out3(t / 0.7));
    wedges.forEach(function (w, i) { prismAim(w, c, out + (i - 3) * 0.09 * (0.3 + 0.7 * open), far, far * 0.11, 0.3 + i * 0.004); w.material.opacity = 0.8 * open; });
    tintDie(die, PRISM_WHITE, 0.5 * open);
    if (t >= 0.5 && !lit) { lit = true; glowNumeral(die, fi, PRISM_WHITE); }
    if (t >= END) { x.end(PRISM_WHITE); return false; }
    return true;
  } };
}
registerCrit('prism', { name: 'Prismatic', kind: 'succ', what: 'A beam of white light strikes the die and splits into a fan of seven colours. The die turns once and sweeps the rainbow round the whole page.', fx: fxPrism,
  beat: 2.2, peak: 3.8, echo: fxPrismEcho, echoWhat: 'Each other 20 throws a small rainbow fan of its own.' });
