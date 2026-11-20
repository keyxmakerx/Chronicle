// ---------- Crit success: Fortune's hand ----------
// Three tarot cards are dealt face down beside the die and turned one at a time: the Star, the Wheel
// of Fortune and, last, the Sun. The Sun's rays come off its card and wash over the die, which lifts
// and takes their gold. The cards rise and scatter into motes.
import {
  dimmer, glowNumeral, inout, out3, registerCrit, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, flat, picture, tarot } from './kit.js';

var FORTUNE_GOLD = '#ffd66b';
var fortuneRays = null;
function fortuneRayTex(T) {
  if (fortuneRays) return fortuneRays;
  fortuneRays = picture(T, 256, 256, function (g) {
    g.translate(128, 128);
    for (var i = 0; i < 20; i++) {
      var a = i / 20 * 6.2832, wd = i % 2 ? 0.028 : 0.05, len = i % 2 ? 96 : 126, gr = g.createLinearGradient(0, 0, Math.cos(a) * len, Math.sin(a) * len);
      gr.addColorStop(0, 'rgba(255,255,255,0)'); gr.addColorStop(0.1, 'rgba(255,255,255,.9)'); gr.addColorStop(0.5, 'rgba(255,255,255,.3)'); gr.addColorStop(1, 'rgba(255,255,255,0)');
      g.fillStyle = gr; g.beginPath(); g.moveTo(0, 0); g.lineTo(Math.cos(a - wd) * len, Math.sin(a - wd) * len); g.lineTo(Math.cos(a + wd) * len, Math.sin(a + wd) * len); g.closePath(); g.fill();
    }
  });
  fortuneRays.userData.shared = true; return fortuneRays;
}
// Where the three cards lie: a fan above the die, the last card in the middle and nearest.
function fortuneSpots(x, up) {
  var s = x.s, c = x.c;
  return [[-2.5, 1.9, 0.3], [2.5, 1.9, -0.3], [0, 2.9, 0]].map(function (p) { return { x: c.x + p[0] * s, z: c.z - p[1] * s * up, ry: p[2] * up }; });
}

function fxFortune(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.4, hd = x.hd;
  var dim = dimmer(st, c, 7.5 * s); x.own.push(dim);
  var names = ['star', 'wheel', 'sun'], flipAt = [1.05, 1.6, 2.25], spots = fortuneSpots(x, 1);
  var cards = names.map(function (n, i) {
    var cd = tarot(T, n, (i === 2 ? 2.7 : 2.3) * s); cd.root.rotation.y = spots[i].ry; x.add(cd.root); return cd;
  });
  var sunAt = new T.Vector3(spots[2].x, 0.3, spots[2].z);
  var raysA = x.add(flat(T, fortuneRayTex(T), 2, 2, { add: true, color: FORTUNE_GOLD, opacity: 0, y: 0.34, order: 8 }));
  var raysB = x.add(flat(T, fortuneRayTex(T), 2, 2, { add: true, color: '#fff3c4', opacity: 0, y: 0.36, order: 8 }));
  var light = new T.PointLight(FORTUNE_GOLD, 0, 9 * s, 1.4); light.position.copy(c).setY(1.6 * s); x.add(light);
  var done = {}, lit = false;
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.42 * smooth(0, 0.7, t) * (1 - smooth(4.3, 5.2, t));
    cards.forEach(function (cd, i) {
      // Dealt from off the top of the page, each a beat behind the last.
      var k = out3((t - 0.12 * i) / 0.7), sp = spots[i], lift = smooth(4.0, 4.9, t);
      cd.root.position.set(sp.x, lift * 1.2 * s, sp.z - (1 - k) * 14 * s);
      cd.root.rotation.y = sp.ry + (1 - k) * (i - 1) * 0.8 + lift * (i - 1) * 0.5;
      cd.turn(inout((t - flipAt[i]) / (i === 2 ? 0.55 : 0.4)));
      cd.fade(1 - smooth(4.2, 4.9, t));
      if (t >= flipAt[i] + 0.3) once('f' + i, function () {
        st.fx.push(spray(st, { n: hd ? 26 : 12, colors: [FORTUNE_GOLD, '#fff3c4'], size: s * 0.05, life: [0.5, 1.0], drag: 1.2, g: -0.3, noFloor: true,
          spawn: function (j, r) { var a = r() * 6.28, v = 0.6 + r() * 1.4; return { p: [sp.x, 0.3, sp.z], v: [Math.cos(a) * v, 0.3 + r() * 0.6, Math.sin(a) * v] }; } }));
      });
    });
    // The Sun shines: its rays leave the card, turn slowly and reach past the die.
    var shine = smooth(2.65, 3.1, t) * (1 - smooth(4.0, 4.8, t)), size = s * (2 + 6.5 * out3((t - 2.65) / 1.3));
    raysA.position.set(sunAt.x, 0.34, sunAt.z); raysB.position.set(sunAt.x, 0.36, sunAt.z);
    raysA.scale.setScalar(size); raysB.scale.setScalar(size * 0.7); raysA.rotation.z = t * 0.35; raysB.rotation.z = -t * 0.5;
    raysA.material.opacity = 0.7 * shine; raysB.material.opacity = 0.5 * shine;
    light.intensity = 26 * shine;
    if (t >= 2.7) once('wave', function () { st.fx.push(wave(st, { x: sunAt.x, z: sunAt.z }, FORTUNE_GOLD, s * 0.8, s * 7, 1.1)); });
    // The die lifts into the light, turns once and comes down gold.
    var up = smooth(2.8, 3.4, t) * (1 - smooth(4.0, 4.7, t));
    x.g.position.set(x.p0.x, x.p0.y + up * 0.7 * s, x.p0.z);
    tintDie(die, FORTUNE_GOLD, 0.7 * shine);
    if (t >= 3.2 && !lit) { lit = true; glowNumeral(die, fi, FORTUNE_GOLD);
      st.fx.push(wave(st, { x: c.x, z: c.z }, '#fff3c4', s * 0.6, s * 3.4, 0.8));
      st.fx.push(spray(st, { n: hd ? 90 : 40, colors: [FORTUNE_GOLD, '#fff3c4', '#ffb84a'], size: s * 0.06, life: [0.9, 1.8], drag: 0.9, g: -0.5, noFloor: true,
        spawn: function (j, r) { var a = r() * 6.28, v = 0.8 + r() * 2.4; return { p: [c.x, x.p0.y + 0.5 * s, c.z], v: [Math.cos(a) * v, 0.4 + r() * 1.2, Math.sin(a) * v], delay: r() * 0.3 }; } })); }
    if (t >= 4.2) once('motes', function () { spots.forEach(function (sp) {
      st.fx.push(spray(st, { n: hd ? 50 : 22, colors: [FORTUNE_GOLD, '#fff3c4'], size: s * 0.05, life: [0.8, 1.5], drag: 0.6, g: -0.9, noFloor: true,
        spawn: function (j, r) { return { p: [sp.x + (r() - 0.5) * 1.3 * s, 0.2 + r() * s, sp.z + (r() - 0.5) * 2.2 * s], v: [(r() - 0.5) * 0.6, 0.5 + r() * 1.2, (r() - 0.5) * 0.6], delay: r() * 0.5 }; } })); }); });
    if (t >= END) { x.end(FORTUNE_GOLD); return false; }
    return true;
  } };
}
// One small card beside the die: it turns up the Star, glints, and is gone.
function fxFortuneEcho(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 1.9, cd = tarot(T, 'star', 1.7 * s), burst = false;
  var a = x.rnd() * 6.28, at = { x: c.x + Math.cos(a) * 1.9 * s, z: c.z + Math.sin(a) * 1.9 * s };
  cd.root.rotation.y = (x.rnd() - 0.5) * 0.8; x.add(cd.root);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var k = out3(t / 0.4);
    cd.root.position.set(at.x, smooth(1.3, 1.8, t) * s, at.z); cd.root.scale.setScalar(0.4 + 0.6 * k);
    cd.turn(inout((t - 0.45) / 0.35)); cd.fade(k * (1 - smooth(1.3, 1.8, t)));
    tintDie(die, FORTUNE_GOLD, 0.5 * smooth(0.7, 1.0, t) * (1 - smooth(1.3, 1.8, t)));
    if (t >= 0.85 && !burst) { burst = true; glowNumeral(die, fi, FORTUNE_GOLD);
      st.fx.push(spray(st, { n: x.hd ? 24 : 10, colors: [FORTUNE_GOLD, '#fff3c4'], size: s * 0.05, life: [0.5, 1.0], drag: 1, g: -0.4, noFloor: true,
        spawn: function (j, r) { var b = r() * 6.28, v = 0.5 + r() * 1.3; return { p: [at.x, 0.3, at.z], v: [Math.cos(b) * v, 0.4 + r() * 0.6, Math.sin(b) * v] }; } })); }
    if (t >= END) { x.end(FORTUNE_GOLD); return false; }
    return true;
  } };
}
registerCrit('fortune', { name: "Fortune's hand", kind: 'succ', what: 'Three tarot cards are dealt beside the die and turned one by one: the Star, the Wheel of Fortune, and last the Sun, whose rays wash over the die and leave the 20 gold.', fx: fxFortune,
  beat: 2.7, peak: 3.6, echo: fxFortuneEcho, echoWhat: 'A single card lands beside each other 20 and turns up the Star.' });
