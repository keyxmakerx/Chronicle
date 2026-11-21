// ---------- Crit fail: Shackled ----------
// Four chains whip in from the corners of the page and catch the die. An iron band closes round it,
// the chains haul tight three times, and a padlock snaps shut. Then the iron rusts through from the
// far ends and falls away, and the die is left where it was, held to its number.
import {
  dimmer, glowNumeral, out3, registerCrit, shadeDie, smooth, spray, tintDie, wave
} from '../engine.js';
import { critScene, flat, picture } from './kit.js';

var SHACKLE_RED = '#ff5a3a', SHACKLE_IRON = '#7d828c', SHACKLE_RUST = '#8a4a22';
// A padlock seen from the front, lying on the page.
function shackleLock(T) {
  return picture(T, 160, 200, function (g) {
    g.lineWidth = 18; g.strokeStyle = '#5c6068'; g.lineCap = 'round'; g.beginPath(); g.moveTo(46, 96); g.lineTo(46, 62); g.arc(80, 62, 34, Math.PI, 0); g.lineTo(114, 96); g.stroke();
    g.lineWidth = 5; g.strokeStyle = 'rgba(255,255,255,.35)'; g.beginPath(); g.arc(80, 62, 39, Math.PI * 1.1, Math.PI * 1.6); g.stroke();
    var gr = g.createLinearGradient(0, 88, 0, 190); gr.addColorStop(0, '#8a8f99'); gr.addColorStop(0.5, '#555a63'); gr.addColorStop(1, '#2f3238');
    g.fillStyle = gr; g.beginPath(); g.roundRect(22, 88, 116, 100, 16); g.fill(); g.lineWidth = 4; g.strokeStyle = '#1c1e22'; g.stroke();
    g.fillStyle = '#16171a'; g.beginPath(); g.arc(80, 128, 12, 0, 6.2832); g.fill(); g.beginPath(); g.moveTo(73, 132); g.lineTo(87, 132); g.lineTo(91, 164); g.lineTo(69, 164); g.closePath(); g.fill();
    g.fillStyle = 'rgba(255,255,255,.3)'; [[34, 100], [126, 100], [34, 176], [126, 176]].forEach(function (p) { g.beginPath(); g.arc(p[0], p[1], 4.5, 0, 6.2832); g.fill(); });
  });
}
// The chains: every link of all of them is one instance, so a chain costs no more than a mesh.
// set(i, from, to, slack, shown, rust) lays chain i from its anchor to its head.
function shackleChains(T, x, count, links, s) {
  var geo = new T.TorusGeometry(0.17 * s, 0.05 * s, 6, 14); geo.scale(1.45, 1, 1);
  var mesh = new T.InstancedMesh(geo, new T.MeshStandardMaterial({ color: '#ffffff', roughness: 0.38, metalness: 0.85, emissive: new T.Color('#14161a'), emissiveIntensity: 1 }), count * links);
  mesh.frustumCulled = false; mesh.castShadow = true; x.add(mesh);
  var m4 = new T.Matrix4(), q = new T.Quaternion(), e = new T.Euler(0, 0, 0, 'YXZ'), p = new T.Vector3(), sc = new T.Vector3(), iron = new T.Color(SHACKLE_IRON), rusty = new T.Color(SHACKLE_RUST), col = new T.Color();
  return { mesh: mesh, set: function (i, from, to, slack, shown, rust, t) {
    var dx = to.x - from.x, dz = to.z - from.z, L = Math.hypot(dx, dz) || 1, ang = Math.atan2(dz, dx), nx = -dz / L, nz = dx / L;
    for (var j = 0; j < links; j++) {
      var u = j / (links - 1), sway = Math.sin(u * Math.PI) * Math.sin(u * 9 - t * 13 + i * 2) * slack, lift = Math.sin(u * Math.PI) * slack * 0.6;
      // Rust travels in from the anchor; a link that has rusted through falls away.
      var r = Math.max(0, Math.min(1, rust * 1.6 - u * 0.6)), gone = Math.max(0, Math.min(1, rust * 2.4 - 1.1 - u * 0.5));
      p.set(from.x + dx * u + nx * sway, to.y + lift + (from.y - to.y) * (1 - u), from.z + dz * u + nz * sway);
      e.set(j % 2 ? Math.PI / 2 : 0.12, -ang, 0); q.setFromEuler(e);
      sc.setScalar(u >= 1 - shown ? Math.max(0.001, 1 - gone) : 0.001); m4.compose(p, q, sc); mesh.setMatrixAt(i * links + j, m4);
      col.copy(iron).lerp(rusty, r); mesh.setColorAt(i * links + j, col);
    }
    mesh.instanceMatrix.needsUpdate = true; if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true;
  } };
}

function fxShackle(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 5.0, hd = x.hd, rnd = x.rnd;
  var dim = dimmer(st, c, 9 * s); x.own.push(dim);
  var NC = 4, LINKS = hd ? 34 : 22, FAR = 12 * s, chains = shackleChains(T, x, NC, LINKS, s), a0 = Math.PI / 4 + (rnd() - 0.5) * 0.3;
  var ends = []; for (var i = 0; i < NC; i++) { var a = a0 + i * Math.PI / 2 + (rnd() - 0.5) * 0.25; ends.push({ a: a, from: new T.Vector3(c.x + Math.cos(a) * FAR, 0.12 * s, c.z + Math.sin(a) * FAR), to: new T.Vector3(), at: 0.2 + i * 0.16 }); }
  // The band round the die's middle, and the lock that hangs from it.
  var band = new T.Mesh(new T.TorusGeometry(1.12 * s, 0.1 * s, 8, 44), new T.MeshStandardMaterial({ color: SHACKLE_IRON, roughness: 0.4, metalness: 0.85, emissive: new T.Color('#14161a'), emissiveIntensity: 1 }));
  band.rotation.x = Math.PI / 2; band.position.set(c.x, x.p0.y * 0.9, c.z); band.castShadow = true; band.visible = false; x.add(band);
  var lock = x.add(flat(T, shackleLock(T), 0.86 * s, 1.08 * s, { opacity: 0, y: x.p0.y + s * 1.1, order: 9 }));
  var iron = new T.Color(SHACKLE_IRON), rusty = new T.Color(SHACKLE_RUST), done = {}, lit = false, PULLS = [1.9, 2.3, 2.7];
  function once(k, fn) { if (!done[k]) { done[k] = 1; fn(); } }
  function sparks(at, n, col) {
    st.fx.push(spray(st, { n: n, colors: col || ['#fff3c4', '#ffb84a', '#ffffff'], size: s * 0.055, life: [0.25, 0.7], drag: 1.2, g: 6,
      spawn: function (j, r) { var b = r() * 6.28, v = 1.5 + r() * 4.5; return { p: [at.x, at.y, at.z], v: [Math.cos(b) * v, 0.8 + r() * 2.4, Math.sin(b) * v] }; } }));
  }
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    dim.material.opacity = 0.42 * smooth(0, 0.6, t) * (1 - smooth(4.3, 5.0, t));
    // Each haul squeezes the die a little further and holds it there.
    var squeeze = 0, jolt = 0;
    PULLS.forEach(function (pt) { squeeze += smooth(pt, pt + 0.08, t) / PULLS.length; jolt += Math.exp(-Math.max(0, t - pt) * 9) * (t >= pt ? 1 : 0); });
    var rust = smooth(3.3, 4.5, t), free = out3((t - 4.25) / 0.4), held = squeeze * (1 - free), sh = s * 0.05 * jolt;
    x.g.position.set(x.p0.x + (rnd() - 0.5) * sh, x.p0.y, x.p0.z + (rnd() - 0.5) * sh); x.g.scale.setScalar(1 - 0.13 * held);
    shadeDie(die, 1 - 0.42 * smooth(1.0, 1.4, t) * (1 - free)); tintDie(die, '#3a4660', 0.3 * held);
    ends.forEach(function (en, i) {
      var reach = out3((t - en.at) / 0.55), grip = (1.02 - 0.13 * held) * s, hx = c.x + Math.cos(en.a) * grip, hz = c.z + Math.sin(en.a) * grip;
      en.to.set(en.from.x + (hx - en.from.x) * reach, x.p0.y * 0.9, en.from.z + (hz - en.from.z) * reach);
      var taut = smooth(en.at + 0.5, 1.5, t), slack = s * (1.3 * (1 - taut) + 0.1 * jolt + 0.03);
      chains.set(i, en.from, en.to, slack, 1, rust, t);
      if (t >= en.at + 0.5) once('hit' + i, function () { sparks(en.to, hd ? 26 : 12); st.fx.push(wave(st, { x: en.to.x, z: en.to.z }, '#cfd6e6', s * 0.2, s * 1.4, 0.3)); });
    });
    var closed = out3((t - 1.2) / 0.3); band.visible = closed > 0.01 && free < 1; band.scale.setScalar((1.7 - 0.7 * closed) * (1 - 0.13 * held) * (1 + 0.5 * free)); band.material.color.copy(iron).lerp(rusty, smooth(3.6, 4.3, t)); band.material.opacity = 1 - free; band.material.transparent = free > 0;
    // The lock drops on at the last haul, sits a moment, and rusts off with the rest.
    var on = out3((t - 2.62) / 0.16); lock.position.set(c.x, x.p0.y + s * 1.1, c.z + s * (1.42 - 0.13 * held)); lock.scale.setScalar(1.5 - 0.5 * on); lock.rotation.z = 0.12 * Math.sin(t * 5) * Math.exp(-Math.max(0, t - 2.7) * 3);
    lock.material.opacity = on * (1 - smooth(4.1, 4.4, t)); lock.material.color.copy(iron).lerp(rusty, smooth(3.6, 4.3, t)).lerp(new T.Color('#ffffff'), 1 - smooth(3.4, 4.2, t));
    if (t >= 1.45) once('band', function () { st.fx.push(wave(st, { x: c.x, z: c.z }, '#9aa6c4', s * 2.6, s * 1.1, 0.35, true)); });
    PULLS.forEach(function (pt, i) { if (t >= pt) once('pull' + i, function () {
      st.fx.push(wave(st, { x: c.x, z: c.z }, i === 2 ? SHACKLE_RED : '#cfd6e6', s * 1.5, s * (2.6 + i * 0.9), 0.45));
      if (i === 2) { sparks({ x: c.x, y: x.p0.y + s, z: c.z + s * 1.3 }, hd ? 40 : 18, [SHACKLE_RED, '#ffd0a0', '#ffffff']); glowNumeral(die, fi, SHACKLE_RED); lit = true; }
    }); });
    if (t >= 3.7) once('rust', function () {
      st.fx.push(spray(st, { n: hd ? 260 : 110, colors: [SHACKLE_RUST, '#b86a30', '#5a3418'], size: s * 0.06, life: [0.6, 1.3], drag: 0.8, g: 5, blend: 'normal',
        spawn: function (j, r) { var en = ends[j % NC], u = r(), d = 0.9 * (1 - u); return { p: [en.from.x + (c.x - en.from.x) * u, x.p0.y * 0.9, en.from.z + (c.z - en.from.z) * u], v: [(r() - 0.5) * 1.2, 0.3 + r() * 0.8, (r() - 0.5) * 1.2], delay: d }; } }));
    });
    if (t >= 4.3) once('free', function () { st.fx.push(wave(st, { x: c.x, z: c.z }, SHACKLE_RED, s * 0.6, s * 3, 0.6)); });
    if (t >= END) { x.end(SHACKLE_RED); return false; }
    return true;
  } };
}
// A band alone closes round the die, squeezes once, and rusts off.
function fxShackleEcho(st, die, fi) {
  var x = critScene(st, die, fi), T = x.T, s = x.s, c = x.c, t = 0, END = 2.0, lit = false, iron = new T.Color(SHACKLE_IRON), rusty = new T.Color(SHACKLE_RUST);
  var band = new T.Mesh(new T.TorusGeometry(1.12 * s, 0.1 * s, 8, 36), new T.MeshStandardMaterial({ color: SHACKLE_IRON, roughness: 0.4, metalness: 0.85, transparent: true, emissive: new T.Color('#14161a'), emissiveIntensity: 1 }));
  band.rotation.x = Math.PI / 2; band.position.set(c.x, x.p0.y * 0.9, c.z); x.add(band);
  return { update: function (dt) {
    if (x.done) return false;
    t += dt;
    var closed = out3(t / 0.3), sq = smooth(0.6, 0.7, t) * (1 - smooth(1.5, 1.8, t)), free = smooth(1.5, 1.9, t);
    band.scale.setScalar((1.7 - 0.7 * closed) * (1 - 0.1 * sq) * (1 + 0.4 * free)); band.material.opacity = 1 - free; band.material.color.copy(iron).lerp(rusty, smooth(1.0, 1.5, t));
    x.g.scale.setScalar(1 - 0.1 * sq); shadeDie(die, 1 - 0.3 * sq);
    if (t >= 0.7 && !lit) { lit = true; glowNumeral(die, fi, SHACKLE_RED); }
    if (t >= END) { x.end(SHACKLE_RED); return false; }
    return true;
  } };
}
registerCrit('shackle', { name: 'Shackled', kind: 'fail', what: 'Four chains whip in from the corners of the page and catch the die. An iron band closes round it, the chains haul tight three times and a padlock snaps shut, before the iron rusts through and falls away.', fx: fxShackle,
  beat: 2.7, peak: 3.5, echo: fxShackleEcho, echoWhat: 'An iron band closes round each other 1, squeezes once, and rusts off.' });
