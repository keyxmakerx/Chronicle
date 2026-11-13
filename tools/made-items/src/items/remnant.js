// Remnant parts: what is left of a creature.
import { THREE, V, M, X, Y, Z, cutStone, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, slab, blob, sheet, fan, rim, chain, many, aim, crystalGeo, lampGlow, wrap, band, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { tex } from '../util.js';
import { metal, bone, leather, cloth, rock, matte, gem, glass, glow, ghost, dark, ivory, shell, furLayers } from '../mats.js';

export function wisp(c, t) {
  const iron = metal(c.trim, { kind: 'forged', rough: .6 }), ph = (t || 0) * Math.PI * 2, n = makeNoise(9, 32);
  const hoop = torus(.62, .065, iron, { r: [Math.PI / 2, 0, 0], p: [0, .066, 0], seg: 64 });
  const studs = []; for (let i = 0; i < 8; i++) { const a = i / 8 * Math.PI * 2; studs.push(ball(.03, iron, { p: [Math.cos(a) * .62, .13, Math.sin(a) * .62], s: [1, .6, 1] })); }
  const ch = chain([[.62, .06, .1], [.9, .05, .3], [1.25, .05, .28], [1.6, .05, .5], [2.0, .05, .45]], .05, iron, { nofit: true });
  // The ghost: a head that trails down into the ring like smoke going the wrong way.
  const spine = []; for (let i = 0; i <= 60; i++) { const s = i / 60, a = s * 5.2 + ph, rr = .3 * Math.pow(1 - s, 1.6) * (s < .12 ? s / .12 : 1); spine.push(V(Math.cos(a) * rr, .08 + s * 1.25 + .02 * Math.sin(ph), Math.sin(a) * rr)); }
  const rad = s => s < .78 ? .012 + .3 * Math.pow(s / .78, 2.1) : .312 * Math.sqrt(Math.max(0, 1 - Math.pow((s - .78) / .22, 2)));
  const shells = [[1, .06], [.74, .09], [.5, .13], [.27, .2]].map(([k, op]) => M(tube(spine, s => Math.max(.001, rad(s) * k * (1 + .22 * (n(s * 7 + k * 5, ph * 2) - .5))), 22), ghost(c.body, op)));
  const trails = []; for (let i = 0; i < 5; i++) { const pts = []; for (let j = 0; j <= 30; j++) { const s = j / 30, a = s * 3.6 + i * 1.26 - ph, rr = .1 + .42 * s * (1 - s) * 2; pts.push(V(Math.cos(a) * rr, .1 + s * (.55 + i * .14), Math.sin(a) * rr)); } trails.push(rod(pts, s => .016 * Math.sin(s * Math.PI) + .002, ghost(c.body, .16), { sides: 6 })); }
  const head = spine[47], heart = ball(.07, ghost(c.detail, .5), { p: [head.x, head.y, head.z] }), thought = ball(.022, glow('#ffffff', 4), { p: [head.x + .03, head.y + .05, head.z + .03] });
  return item('Bound wisp', [part('binding ring', 'trim', [0, -.3, 0], [hoop, ch].concat(studs)), part('wisp', 'body', [0, .2, 0], shells.concat(trails)), part('its heart', 'detail', [.6, .3, 0], [heart]), part('its last thought', 'detail', [-.6, .5, 0], [thought])], { stand: true, yaw: .2, el: 20, lights: [{ p: [0, .9, 0], color: c.body, power: 3 }], halo: .8 });
}

// A curved, tapering solid with a pointed end: teeth, talons. cen and tan give its spine.
function tusk(cen, tan, rf, mat, o) {
  o = o || {}; const N = o.n || 72, n = 40, rings = [], na = makeNoise(o.seed || 77, 32);
  for (let i = 0; i <= N; i++) { const s = i / N, t = tan(s), ax = V(-t.y, t.x, 0), r = rf(s), pts = [];
    for (let j = 0; j < n; j++) { const th = j / n * Math.PI * 2, lump = (fbm(na, Math.cos(th) * 2 + 9, s * 5 + Math.sin(th) * 2, 3) - .5) * (o.lump == null ? .09 : o.lump) * (1 - s * .7), flute = Math.cos(th * 5) * .012 * Math.max(0, 1 - s * 1.6) * (o.flute == null ? 1 : o.flute), keel = Math.pow(Math.max(0, Math.cos(th)), 6) * (o.keel == null ? .06 : o.keel) * s, k = r * (1 + lump + flute + keel); pts.push([Math.cos(th) * k, Math.sin(th) * k * (o.flat || .8)]); }
    rings.push(ring(cen(s), ax, Z, pts)); }
  if (o.ragged) for (let j = 0; j < n; j++) rings[0][j].addScaledVector(tan(0), (fbm(na, j * .9, 3.3, 2) - .5) * .09);
  return { mesh: M(loft(rings), mat), rings: rings };
}

export function fang(c, t) {
  const L = 1.9, cen = s => V(.55 * Math.pow(s, 1.9) - .12, -L * s + .9, 0), tan = s => V(.55 * 1.9 * Math.pow(Math.max(s, 1e-4), .9), -L, 0).normalize(), rf = s => .2 * Math.pow(1 - Math.pow(s, 1.25), .8) * (1 + .1 * Math.exp(-s * 14)) + .0015;
  const tk = tusk(cen, tan, rf, ivory(51), { ragged: true }), gum = matte(c.detail, { rough: .35, wet: .7, bump: .8 }), ven = () => glow(c.trim, 1.5), ph = (t || 0);
  const collar = M(tube(tk.rings[2].map((q, j) => q.clone().add(V(0, .02 + .03 * Math.sin(j * 1.7), 0))), .045, 8, true), gum), shreds = [0, 1, 2, 3, 4].map(i => { const q = tk.rings[1][i * 8 + 2]; return rod([[q.x, q.y, q.z], [q.x * 1.15 + .02, q.y + .12, q.z * 1.2], [q.x * 1.2, q.y + .2 + i * .02, q.z * 1.3]], s => .03 * (1 - s * .8), gum, { n: 8, sides: 6 }); });
  const cap = blob([.19, .06, .15], null, gum, { p: [-.12, .93, 0] });
  const groove = []; for (let i = 0; i <= 20; i++) { const s = .45 + i / 20 * .55, q = cen(s), tt = tan(s); groove.push(V(q.x - (-tt.y) * rf(s) * .2, q.y - tt.x * rf(s) * .2, rf(s) * .8 + .004)); }
  const line = M(tube(groove, s => .012 * (1 - s * .6), 6), ven()), tip = cen(1), drop = ball(.035, ven(), { s: [1, 1.5, 1], p: [tip.x + .005, tip.y - .05, 0] }), fall = ball(.024, ven(), { s: [1, 1.6, 1], p: [tip.x + .005, tip.y - .1 - .28 * ph, 0] });
  return item('Fang', [part('fang', 'body', [0, 0, 0], [tk.mesh]), part('root', 'detail', [0, .4, 0], [collar, cap].concat(shreds)), part('venom', 'trim', [.3, -.3, .2], [line, drop, fall])], { yaw: .62, fill: .9, lights: [{ p: [tip.x, tip.y, .2], color: c.trim, power: .5 }], halo: .7 });
}

export function claw(c) {
  const R = .95, A = 1.75, cen = s => V(R * (1 - Math.cos(s * A)) - .35, -R * Math.sin(s * A) + .3, 0), tan = s => V(Math.sin(s * A), -Math.cos(s * A), 0), rf = s => .21 * Math.pow(1 - Math.pow(s, 1.5), .9) + .002;
  const tk = tusk(cen, tan, rf, bone(c.body, { gloss: 1, rough: .16 }), { lump: .03, flute: .3, keel: .12, flat: .72 }), hideM = leather(c.trim, { bump: 1.6 }), wire = metal(c.detail, { kind: 'cast', rough: .3 });
  const tp = path([[-.35, .26, 0], [-.4, .6, 0], [-.52, .95, 0], [-.7, 1.25, 0]], 24), toe = M(tube(tp, s => .23 + .03 * Math.sin(s * 9), 20), hideM, { s: [1, 1, .85] });
  const sc = []; for (let i = 2; i < 23; i += 2) for (let k = -3; k <= 3; k++) { const q = tp[i], a = k * .42 + (i % 4 ? .2 : 0); sc.push({ p: [q.x + Math.sin(a) * .235, q.y, Math.cos(a) * .2], r: [0, a, 0], s: [1, 1.25, .35] }); }
  const scales = many(new THREE.SphereGeometry(.07, 10, 8), hideM, sc), stump = ball(.24, matte('#5a2026', { wet: .5, rough: .4 }), { s: [1, .5, .85], p: [-.7, 1.26, 0], r: [0, 0, .5] });
  const hel = []; for (let i = 0; i <= 120; i++) { const s = i / 120, a = s * Math.PI * 2 * 6, y = .2 + s * .2; hel.push(V(-.36 + Math.cos(a) * (.235 + .01), y, Math.sin(a) * (.235 + .01) * .82)); }
  const bind = [M(tube(hel, .014, 6), wire), rod([[-.13, .4, .12], [.05, .5, .2], [.12, .62, .16]], .014, wire, { n: 10 })];
  return item('Talon', [part('talon', 'body', [.2, -.3, 0], [tk.mesh]), part('scaled toe', 'trim', [-.2, .4, 0], [toe, scales, stump]), part('binding wire', 'detail', [0, 0, .6], bind)], { yaw: .2, fill: .9 });
}

export function horn(c, t) {
  const n = makeNoise(63, 32), cen = s => { const th = -.6 + s * 3.5, R = .8 * (1 - .5 * s); return V(Math.cos(th) * R, Math.sin(th) * R, s * .5); }, sp = []; for (let i = 0; i <= 90; i++) sp.push(cen(i / 90));
  const rf = s => (.27 * Math.pow(1 - s, .85) + .012) * (1 + .06 * Math.sin(s * 110) + .03 * Math.sin(s * 37));
  const body = M(tube(sp, rf, 24), bone(c.body, { gloss: .5, rough: .4 })), br = metal(c.trim, { kind: 'cast', rough: .35 });
  const mount = M(tube(sp.slice(0, 9), s => .3 - s * .015, 24), br), lip = M(tube(sp.slice(0, 2), .33, 24), br), ringM = M(tube(sp.slice(8, 10), .3, 24), br);
  const t0 = sp[1].clone().sub(sp[0]).normalize(), mouth = M(new THREE.CircleGeometry(.27, 32), dark(), { p: [0, 0, 0] }); mouth.geometry.lookAt(t0.clone().negate()); mouth.geometry.translate(sp[0].x - t0.x * .002, sp[0].y - t0.y * .002, sp[0].z - t0.z * .002);
  const r = rng(3 + Math.floor((t || 0) * 10)), arcs = [];
  for (let k = 0; k < 4; k++) { const s0 = .15 + r() * .6, pts = []; for (let j = 0; j <= 7; j++) { const s = s0 + j * .028, q = cen(Math.min(1, s)), a = k * 1.7 + j * .7; pts.push(V(q.x + Math.cos(a) * (rf(s) + .05) + (r() - .5) * .07, q.y + (r() - .5) * .07, q.z + Math.sin(a) * (rf(s) + .05) + .02)); } arcs.push(M(tube(pts, .009, 5), glow(c.detail, 3))); }
  const hum = M(new THREE.CircleGeometry(.2, 24), glow(c.detail, .22, { add: true })); hum.geometry.lookAt(t0.clone().negate()); hum.geometry.translate(sp[0].x - t0.x * .01, sp[0].y - t0.y * .01, sp[0].z - t0.z * .01);
  return item('Horn', [part('horn', 'body', [0, 0, 0], [body]), part('mount', 'trim', [.5, -.3, 0], [mount, lip, ringM, mouth]), part('what still hums in it', 'detail', [0, 0, .7], arcs)], { yaw: .3, fill: .9, lights: [{ p: [0, .3, .5], color: c.detail, power: .9, flick: .4 }], halo: .8 });
}

export function scale(c, t) {
  const out = [[0, .95], [.34, .72], [.55, .28], [.5, -.3], [.3, -.72], [0, -.98], [-.3, -.72], [-.5, -.3], [-.55, .28], [-.34, .72]];
  const zf = h => (x, y, v) => h * Math.pow(Math.max(0, 1 - v * v), .55) + .07 * Math.exp(-Math.pow(x / .045, 2)) * (1 - v * .8) + .0022 * Math.cos(v * 30) * (1 - v * .5) + .004 * Math.cos(x * 60) * v * (1 - v);
  const main = fan(out, zf(.2), shell(c.body, { metal: .6, rough: .22 }), { nu: 120, nv: 40, uvx: 2, uvy: 2 }), under = [[-.42, .36, -.5], [.44, .3, .45]].map(([x, y, rz]) => { const m = fan(out, zf(.16), shell(c.trim, { metal: .5, rough: .3 }), { nu: 90, nv: 30 }); m.geometry.scale(.82, .82, 1); m.geometry.rotateZ(rz); m.geometry.translate(x, y, -.06); return m; });
  const ph = (t || 0) * Math.PI * 2, pw = 1.3 * (1 + .5 * Math.sin(ph)), spot = ball(.11, glow(c.detail, pw), { s: [1, 1.25, .25], p: [0, -.2, .2] });
  const warm = lampGlow(main, [0, -.2, .22], c.detail, .7 * (1 + .5 * Math.sin(ph)), .16);
  return item('Scale', [part('under scales', 'trim', [0, .3, -.3], under), part('scale', 'body', [0, 0, 0], [main]), part('heat spot', 'detail', [0, 0, .7], [spot, warm])], { yaw: .3, fill: .86, lights: [{ p: [0, -.2, .5], color: c.detail, power: .8 * (1 + .5 * Math.sin(ph)) }], halo: .8 });
}

export function feather(c) {
  const n = makeNoise(52, 32), barbs = tex(256, 256, (u, v) => .5 + .5 * Math.sin((v * 1.0 + u * .55) * 300)), vaneM = side => { const m = cloth(c.body, { sheen: 1, bump: 0 }); m.bumpMap = barbs; m.bumpScale = 1.2; m.iridescence = .8; m.iridescenceIOR = 1.5; m.roughness = .55; return m; };
  const sp = s => V(.16 * Math.sin(s * 2.2) - .1, -1.5 + s * 3.0, .05 * Math.sin(s * 3)), w = v => .42 * Math.pow(Math.sin(Math.min(1, v * 1.04) * Math.PI), .55) * (1 - .25 * v);
  const half = k => sheet((u, v) => { const s = .22 + v * .78, q = sp(s), split = 1 - .5 * Math.max(0, Math.sin(v * 37 + k * 2) - .93) * 12 * u, ww = w(v) * (k > 0 ? 1 : .72) * split; return V(q.x + k * u * ww * Math.cos(.5), q.y - u * ww * .55, q.z + u * .05 - u * u * .1 + (n(v * 20, u * 3 + k) - .5) * .02 * u); }, 14, 120, vaneM(k), { uvx: 1, uvy: 6 });
  const quill = M(tube(Array.from({ length: 50 }, (_, i) => sp(i / 49)), s => .03 * (1 - s * .9) + .003, 10), bone(c.trim, { gloss: .7 }));
  const e = sp(.82), eye = [ball(.13, gem(c.detail, { round: true, edges: 0, lit: .3 }), { s: [1, 1.3, .25], p: [e.x + .03, e.y - .12, e.z + .04] }), torus(.17, .02, cloth('#1a1410'), { s: [1, 1.3, .5], p: [e.x + .03, e.y - .12, e.z + .03] })];
  return item('Feather', [part('vane', 'body', [.2, 0, 0], [half(1), half(-1)]), part('quill', 'trim', [-.3, -.2, 0], [quill]), part('eye spot', 'detail', [0, .3, .6], eye)], { yaw: -.55, fill: .95 });
}

export function eye(c) {
  const white = matte(c.body, { rough: .16, wet: 1 }), nerveM = matte(c.trim, { rough: .3, wet: .8, bump: .8 }), r = rng(21);
  const iris = tex(256, 128, (u, v) => { const d = 1 - v, a = u * Math.PI * 2, col = new THREE.Color(c.detail), k = (.5 + .5 * Math.pow(.5 + .5 * Math.sin(a * 46 + Math.sin(a * 9) * 2.5), 2)) * (.55 + .7 * d) * (d > .9 ? .25 : 1) * (d < .36 ? .5 : 1); return [col.r * k * 1.9, col.g * k * 1.7, col.b * k * 1.5].map(q => Math.pow(Math.max(0, Math.min(1, q)), 1 / 2.2)); }, { srgb: true });
  const grp = [ball(.5, white, { seg: 64, seg2: 44 })];
  const irisM = new THREE.MeshPhysicalMaterial({ map: iris, roughness: .3, emissive: new THREE.Color(c.detail), emissiveIntensity: .1 }); irisM.userData = { label: '' };
  grp.push(M(new THREE.SphereGeometry(.503, 48, 10, 0, Math.PI * 2, 0, .5), irisM, { r: [Math.PI / 2, 0, 0] }));
  grp.push(M(new THREE.SphereGeometry(.507, 24, 8, 0, Math.PI * 2, 0, .36), dark(), { r: [Math.PI / 2, 0, 0], s: [.2, 1, 1] }));
  grp.push(M(new THREE.SphereGeometry(.34, 40, 16, 0, Math.PI * 2, 0, .95), glass('#ffffff', { tint: .03 }), { r: [Math.PI / 2, 0, 0], p: [0, 0, .215] }));
  const veins = []; for (let i = 0; i < 9; i++) { const a = r() * 6.28, pts = []; let th = 2.4 + r() * .3; for (let j = 0; j < 6; j++) { th -= .22; const aa = a + (r() - .5) * .3 * j; pts.push(V(Math.sin(th) * Math.cos(aa) * .503, Math.sin(th) * Math.sin(aa) * .503, Math.cos(th) * .503)); } veins.push(M(tube(path(pts, 14), s => .007 * (1 - s * .8), 5), matte('#8a2a30', { wet: .6 }))); }
  const tilt = new THREE.Matrix4().makeRotationFromEuler(new THREE.Euler(-.25, .3, 0)); grp.concat(veins).forEach(m => { m.geometry.applyMatrix4(tilt); m.geometry.translate(0, .5, 0); });
  const nerve = rod([[0, .5, -.45], [-.2, .3, -.75], [-.6, .09, -.9], [-1.0, .07, -.7], [-1.2, .06, -.3]], s => .11 * (1 - s * .5), nerveM, { n: 40, sides: 12 }), strands = [0, 1].map(i => rod([[.1 * (i ? 1 : -1), .45, -.45], [.3 * (i ? 1 : -1), .2, -.7], [.5 * (i ? 1 : -1.6), .03, -.8 + i * .3]], s => .03 * (1 - s * .6), nerveM, { n: 20, sides: 8 }));
  return item('Watching eye', [part('eye', 'body', [0, 0, 0], [grp[0]].concat(veins)), part('iris', 'detail', [0, 0, .8], grp.slice(1)), part('nerve', 'trim', [-.4, 0, -.5], [nerve].concat(strands))], { stand: true, yaw: .35, el: 20 });
}

export function core(c, t) {
  const n = makeNoise(88, 32), slag = rock(c.trim, { bump: 2.4, rough: .8 }), ph = (t || 0) * Math.PI * 2, pw = 1 + .18 * Math.sin(ph), r = rng(5);
  const fire = tex(256, 256, (u, v) => { const k = fbm(n, u * 5, v * 5, 4), ridge = 1 - Math.abs(2 * k - 1), col = new THREE.Color(c.body), hot = new THREE.Color(c.detail), f = Math.pow(ridge, 7) * 2.6 + Math.pow(k, 3) * .5; return [Math.min(1, col.r * .5 + hot.r * f), Math.min(1, col.g * .16 + hot.g * f), Math.min(1, col.b * .1 + hot.b * f * .7)].map(q => Math.pow(q, 1 / 2.2)); }, { srgb: true });
  const inner = new THREE.MeshBasicMaterial({ map: fire, color: new THREE.Color(1.5 * pw, 1.5 * pw, 1.5 * pw) }); inner.userData = { glow: true, noshadow: true, label: '' };
  const ballM = ball(.5, inner, { p: [0, .62, 0], seg: 48 });
  // The shell: one cooled crust, split along cracks. A second crust under it makes each crack a step down to the fire.
  const pts = Array.from({ length: 46 }, () => { const z = r() * 2 - 1, a = r() * 6.283, q = Math.sqrt(1 - z * z); return [Math.cos(a) * q, z, Math.sin(a) * q]; });
  const crack = tex(512, 256, (u, v) => { const th = v * Math.PI, a = u * Math.PI * 2, x = Math.sin(th) * Math.cos(a), y = Math.cos(th), z = Math.sin(th) * Math.sin(a); let d1 = 9, d2 = 9; for (const q of pts) { const d = Math.hypot(x - q[0], y - q[1], z - q[2]); if (d < d1) { d2 = d1; d1 = d; } else if (d < d2) d2 = d; } const gap = d2 - d1, hole = fbm(n, x * 1.6 + 4, y * 1.6 + z * 1.6, 3); return Math.max(0, Math.min(1, (gap - .012 - Math.max(0, hole - .66) * .8) * 9)); });
  const crustM = rock(c.trim, { bump: 2.6, rough: .85 }); crustM.alphaMap = crack; crustM.alphaTest = .52; crustM.side = THREE.DoubleSide;
  const underM = rock(c.trim, { bump: 1.6, rough: .9 }); underM.alphaMap = crack; underM.alphaTest = .2; underM.side = THREE.DoubleSide; underM.color.multiplyScalar(.35); underM.emissive = new THREE.Color(c.body); underM.emissiveIntensity = .5;
  const lump = v => 1 + (fbm(n, v.x * 3 + 1, v.y * 3 + v.z * 3, 3) - .5) * .09;
  const plates = [blob([.575, .575, .575], lump, crustM, { p: [0, .62, 0], seg: 96, seg2: 64 }), blob([.535, .535, .535], lump, underM, { p: [0, .62, 0], seg: 96, seg2: 64 })], loose = [];
  for (let i = 0; i < 6; i++) { const a = r() * 6.28, d = .7 + r() * .3; loose.push(M(new THREE.DodecahedronGeometry(.06 + r() * .06, 0), slag, { r: [r() * 3, r() * 3, 0], p: [Math.cos(a) * d, .05, Math.sin(a) * d] })); }
  const sparks = []; for (let i = 0; i < 16; i++) { const a = r() * 6.28, d = .35 + r() * .5, h = ((r() + (t || 0)) % 1), m = ball(.012 + r() * .012, glow(c.detail, 2.4 * (1 - h * .8)), { p: [Math.cos(a) * d * (1 - h * .3), .5 + h * 1.0, Math.sin(a) * d * (1 - h * .3)], seg: 8, seg2: 6 }); m.userData.nofit = true; sparks.push(m); }
  return item('Elemental core', [part('core', 'body', [0, 0, 0], [ballM]), part('shell', 'trim', [0, .1, .9], plates), part('loose pieces', 'trim', [.6, -.1, .3], loose), part('heat haze', 'detail', [0, .5, 0], sparks)], { stand: true, yaw: .3, el: 18, lights: [{ p: [0, .62, 0], color: c.detail, power: 3, flick: .15 }], halo: .75, spread: .5 });
}

export function heart(c, t) {
  const n = makeNoise(46, 32), flesh = matte(c.body, { rough: .38, wet: .55, bump: 1.1 }), vess = matte(c.trim, { rough: .34, wet: .7, bump: .5 }), gm = gem(c.detail, { vc: true, lit: .35, inner: .5 }), r = rng(4), S = [.6, .72, .46];
  const g = V(.78, .3, -.5).normalize(), e1 = V(-.36, .93, 0).normalize(), e2 = new THREE.Vector3().crossVectors(g, e1);
  // A blunt cone below, two swellings above, and the groove where the chambers meet.
  const f = v => { const th = Math.acos(Math.max(-1, Math.min(1, -v.y))), cone = v.y < 0 ? 1 / (Math.sin(th) + .9 * Math.cos(th)) : 1, low = 1 + (cone - 1) * .78, top = v.y > 0 ? 1 + .16 * v.y * Math.abs(v.x) - .2 * v.y * Math.exp(-Math.pow((v.x - .1) / .3, 2)) - .12 * v.y * v.y : 1; return low * top - .055 * Math.exp(-Math.pow(v.dot(g) / .07, 2)) + (fbm(n, v.x * 3 + 3, v.y * 3 + v.z * 2, 3) - .5) * .09; };
  const on = (v, lift) => { const k = f(v) + (lift || 0); return V(v.x * S[0] * k, v.y * S[1] * k, v.z * S[2] * k); }, arc = (a0, a1, m, w) => Array.from({ length: m + 1 }, (_, i) => { const a = a0 + (a1 - a0) * i / m, v = e1.clone().multiplyScalar(Math.cos(a)).addScaledVector(e2, Math.sin(a)).addScaledVector(g, (w || 0) * Math.sin(i / m * Math.PI)).normalize(); return on(v, .012); });
  const body = blob(S, f, flesh, { seg: 120, seg2: 80 });
  const veins = [rod(arc(.25, 2.9, 30), s => .03 * (1 - s * .7), vess, { sides: 8 }), rod(arc(1.0, 2.2, 14, .5), s => .018 * (1 - s * .6), vess, { sides: 6 }), rod(arc(1.5, 2.6, 14, -.45), s => .016 * (1 - s * .6), vess, { sides: 6 }), rod(arc(.6, 1.5, 12, -.3), s => .014 * (1 - s * .6), vess, { sides: 6 })];
  const cut = (p, dir, rr) => { const q = aim(V(dir[0], dir[1], dir[2])), a = torus(rr, .03, vess, { r: [Math.PI / 2, 0, 0] }), b = cyl(rr * .92, rr * .92, .02, dark()); for (const m of [a, b]) { m.geometry.applyQuaternion(q); m.geometry.translate(p[0], p[1], p[2]); } return [a, b]; };
  const pipes = [rod([[0, .5, .04], [.02, .9, .05], [-.12, 1.1, .03], [-.36, 1.1, 0], [-.52, .94, -.04], [-.56, .7, -.08]], .135, vess, { n: 40, sides: 16 }), rod([[.24, .46, .2], [.14, .76, .25], [-.06, .94, .22], [-.2, 1.0, .2]], .11, vess, { n: 24, sides: 14 }), rod([[.4, .42, -.04], [.46, .78, -.04], [.5, 1.04, -.04]], .1, vess, { n: 14, sides: 14 })]
    .concat([[[-.02, 1.1, .04], [.02, 1.34, .06]], [[-.2, 1.16, .02], [-.2, 1.38, .02]], [[-.37, 1.12, 0], [-.44, 1.3, -.02]]].map(q => rod(q, .05, vess, { sides: 10 })))
    .concat(cut([-.56, .7, -.08], [-.05, -1, -.1], .135), cut([-.2, 1.0, .2], [-1, .35, -.1], .11), cut([.5, 1.04, -.04], [.1, 1, 0], .1), cut([.02, 1.34, .06], [.15, 1, .08], .05), cut([-.2, 1.38, .02], [0, 1, 0], .05), cut([-.44, 1.3, -.02], [-.35, 1, 0], .05));
  const grow = [[-.2, .08, .36, 1.1, .5, .1, .58], [-.32, .0, .3, .8, 1.0, .07, .4], [-.1, -.06, .36, 1.3, -.2, .06, .36], [.3, -.2, .3, 1.0, -.7, .09, .5], [.36, -.3, .24, .7, -1.2, .06, .34], [.24, .34, .34, 1.2, -.3, .06, .3], [-.06, -.52, .2, 1.0, .2, .05, .3], [.2, -.12, .34, 1.45, .2, .045, .26]].map((q, i) => M(crystalGeo(q[5], q[6], q[6] * .32, { seed: i + 3, hex: c.detail }), gm, { r: [q[3], 0, q[4]], p: [q[0], q[1], q[2]] }));
  const all = [body].concat(veins, pipes, grow), fit = new THREE.Box3(), tt = ((t || 0) * 2) % 1, thump = Math.exp(-Math.pow((tt - .12) / .06, 2)) + .6 * Math.exp(-Math.pow((tt - .36) / .07, 2)), k = 1 + .045 * thump;
  // It still beats. The frame is measured at rest so the beat shows as swelling, not as the camera pulling back.
  for (const m of all) { m.geometry.rotateZ(.3); m.geometry.computeBoundingBox(); fit.union(m.geometry.boundingBox); m.geometry.scale(k, k, 1 + (k - 1) * .5); }
  return item('Crystal heart', [part('heart', 'body', [0, 0, 0], [body].concat(veins)), part('great vessels', 'trim', [0, .5, 0], pipes), part('growths', 'detail', [0, 0, .8], grow)], { yaw: 0, fill: .86, fit: fit });
}

export function tentacle(c, t) {
  const n = makeNoise(57, 32), flesh = matte(c.body, { rough: .22, wet: 1, bump: 1.2 }), cup = matte(c.trim, { rough: .3, wet: .8 }), sp = [], N = 120;
  for (let i = 0; i <= N; i++) { const s = i / N, th = .2 + s * 6.4, R = 1.0 * Math.pow(1 - s, .8) + .06; sp.push(V(Math.cos(th) * R - .1, Math.sin(th) * R, .22 * Math.pow(1 - s, .85) + .01)); }
  const rf = s => (.22 * Math.pow(1 - s, .85) + .012) * (1 + .05 * Math.sin(s * 60));
  const body = M(tube(sp, rf, 22), flesh), cups = [], cupIn = [];
  for (let i = 4; i < N - 6; i += 3) { const s = i / N, q = sp[i], tt = sp[i + 1].clone().sub(sp[i - 1]).normalize(), inward = V(-q.x - .1, -q.y, 0).normalize(), d = inward.clone().multiplyScalar(.75).add(V(0, 0, .66)).normalize(), rr = rf(s); for (const k of [-1, 1]) { const pos = q.clone().addScaledVector(d, rr * .98).addScaledVector(tt, 0).addScaledVector(V(0, 0, 1).cross(d).normalize(), 0); const side = d.clone().applyAxisAngle(tt, k * .42); const pp = q.clone().addScaledVector(side, rr * .99); cups.push({ p: pp, q: new THREE.Quaternion().setFromUnitVectors(Z, side), s: rr * 1.5 }); } }
  const cupsM = many(new THREE.TorusGeometry(.12, .05, 8, 16), cup, cups), cupsD = many(new THREE.CircleGeometry(.1, 12), matte('#3a2a3a', { wet: .5 }), cups.map(q => ({ p: q.p.clone().add(V(0, 0, 0)), q: q.q, s: q.s })));
  const base = sp[0], stump = ball(.2, matte('#6a2a4a', { wet: .8, rough: .3 }), { s: [1, 1, 1], p: [base.x + .0, base.y - .03, base.z] });
  const ph = (t || 0), drops = [[.0, -.12, .0], [.12, -.2, -.08], [-.1, -.26, -.1]].map(([x, y, z], i) => ball(.04 + i * .008, glow(c.detail, 1.4 * (1 + .45 * Math.sin(ph * Math.PI * 2 + i * 2.1))), { s: [1, .6, 1], p: [base.x + x, base.y + y - .1, .03] })), fall = ball(.034 * (1 - ph * .5), glow(c.detail, 1.6), { s: [1, 1.4, 1], p: [base.x + .02, base.y - .3 - .5 * ph * ph, .03] });
  return item('Tendril', [part('tendril', 'body', [0, 0, 0], [body, stump]), part('cups', 'trim', [0, 0, .6], [cupsM, cupsD]), part('what it leaks', 'detail', [.4, -.4, 0], drops.concat([fall]))], { yaw: .2, fill: .88, lights: [{ p: [base.x, base.y - .2, .3], color: c.detail, power: .5 }], halo: .7 });
}

export function carapace(c) {
  const out = [[0, 1.0], [.5, .9], [.86, .5], [.95, 0], [.8, -.55], [.45, -.92], [0, -1.02], [-.45, -.92], [-.8, -.55], [-.95, 0], [-.86, .5], [-.5, .9]], n = makeNoise(72, 32);
  const zf = (x, y, v) => .42 * Math.pow(Math.max(0, 1 - v * v), .5) * (1 - .15 * y) + .035 * (1 - Math.abs(((y * 2.6 + 9) % 1) - .5) * 2) * (1 - v * v) + .04 * Math.exp(-Math.pow(x / .06, 2)) * (1 - v) + (n(x * 5, y * 5) - .5) * .015;
  const plate = fan(out, zf, shell(c.body, { metal: .45, rough: .24 }), { nu: 160, nv: 44, uvx: 3, uvy: 2 }), sp = bone(c.trim, { gloss: .5 });
  const spines = []; const cc = new THREE.CatmullRomCurve3(out.map(p => V(p[0], p[1], 0)), true);
  for (let i = 0; i < 18; i++) { const u = i / 18, q = cc.getPointAt(u), d = V(q.x, q.y, .1).normalize(), len = .16 + .14 * Math.abs(Math.sin(i * 2.3)); spines.push({ p: q.clone().multiplyScalar(.97).add(V(0, 0, .03)).addScaledVector(d, len * .5), q: aim(d), s: [1, len / .2, 1] }); }
  const spinesM = many(new THREE.ConeGeometry(.045, .2, 8), sp, spines);
  const spots = [[-.4, .4], [.42, .36], [0, .62], [-.5, -.2], [.5, -.24], [-.2, -.6], [.24, -.62], [0, .0]].map(([x, y]) => ball(.07, gem(c.detail, { round: true, edges: 0, lit: .3 }), { s: [1, 1, .3], p: [x, y, zf(x, y, Math.hypot(x, y) / .95) + .0] }));
  return item('Carapace plate', [part('plate', 'body', [0, 0, 0], [plate]), part('spines', 'trim', [0, 0, -.4], [spinesM]), part('sheen spots', 'detail', [0, 0, .7], spots)], { yaw: .25, fill: .88 });
}

export function knot(c, t) {
  const a = metal(c.body, { kind: 'cast', rough: .22 }), b = metal(c.trim, { kind: 'cast', rough: .22 }), ph = (t || 0) * Math.PI * 2;
  // Each loop turns through itself; a knot wound q times looks the same after a 1/q turn, so the clip closes.
  const g1 = new THREE.TorusKnotGeometry(.52, .1, 220, 20, 2, 3), g2 = new THREE.TorusKnotGeometry(.5, .07, 220, 16, 3, 2); g1.rotateZ(ph / 3); g2.rotateZ(-ph / 2);
  const k1 = M(g1, a, { p: [0, .8, 0], r: [.3, 0, 0] }), k2 = M(g2, b, { p: [0, .8, 0], r: [0, .8, .4] });
  const front = M(new THREE.TorusGeometry(.3, .09, 16, 48), a, { r: [1.2, .4, 0], p: [.1, .75, .3] });
  const voidM = ball(.24, new THREE.MeshBasicMaterial({ color: 0x000000 }), { p: [0, .8, 0] }); voidM.material.userData = { label: '', noshadow: true };
  const wrong = [ball(.36, ghost(c.detail, .14), { p: [0, .8, 0] }), ball(.6 + .03 * Math.sin(ph), ghost(c.detail, .05), { p: [0, .8, 0], s: [1.1, .9, 1] }), ball(.9 + .04 * Math.cos(ph), ghost(c.detail, .025), { p: [.05, .82, 0], s: [1, 1.1, .9] })];
  return item('Nameless thing', [part('a loop', 'body', [-.4, 0, 0], [k1]), part('a loop that goes through it', 'trim', [.4, 0, 0], [k2]), part('a part that should not be in front', 'body', [0, 0, .8], [front]), part('the wrongness around it', 'detail', [0, .6, 0], wrong.concat([voidM]))], { stand: true, yaw: .3, el: 18, lights: [{ p: [0, .8, 0], color: c.detail, power: 1.2 }], halo: .8 });
}

export function pelt(c) {
  const n = makeNoise(95, 32), out = [[0, 1.3], [.22, 1.2], [.34, .95], [.7, .98], [1.02, 1.12], [1.1, .86], [.82, .62], [.6, .3], [.62, -.3], [.86, -.66], [1.1, -.9], [1.0, -1.16], [.68, -1.02], [.36, -.86], [.14, -1.0], [.1, -1.45], [0, -1.6], [-.1, -1.45], [-.14, -1.0], [-.36, -.86], [-.68, -1.02], [-1.0, -1.16], [-1.1, -.9], [-.86, -.66], [-.62, -.3], [-.6, .3], [-.82, .62], [-1.1, .86], [-1.02, 1.12], [-.7, .98], [-.34, .95], [-.22, 1.2]];
  const stripe = (u, v) => { const x = (u - .5) * 2.4, y = (v - .5) * 3.2, s = Math.sin(y * 9 + Math.sin(x * 3) * 1.6 + n(x * 2 + 5, y * 2) * 5); return Math.max(0, Math.min(1, (s - .35) * 5)) * Math.min(1, Math.abs(x) * 2.2 + .15); };
  const mats = furLayers(c.body, c.detail, 16, stripe), zf = (x, y) => .03 + (n(x * 1.5 + 2, y * 1.5) - .5) * .09, layers = [];
  for (let i = 0; i < 16; i++) { const m = fan(out, (x, y) => zf(x, y) + i * .0065, mats[i], { nu: 160, nv: 30, tension: .3 }); const p = m.geometry.attributes.position, uv = m.geometry.attributes.uv; for (let k = 0; k < p.count; k++) uv.setXY(k, p.getX(k) / 2.4 + .5, p.getY(k) / 3.2 + .5); if (i) { m.userData.noshadow = true; m.userData.nofit = true; } layers.push(m); }
  const under = fan(out.map(q => [q[0] * 1.03, q[1] * 1.03]), (x, y) => zf(x, y) - .012, leather(c.trim, { double: true }), { nu: 160, nv: 8, tension: .3 });
  return item('Shifting pelt', [part('hide', 'trim', [0, 0, -.3], [under]), part('fur', 'body', [0, 0, .2], layers)], { yaw: .2, fill: .9 });
}

export function skull(c, t) {
  const bn = ivory(33, { stain: .95 }), hornM = bone(c.trim, { gloss: .5, rough: .4 }), n = makeNoise(83, 32), ph = (t || 0) * Math.PI * 2, pw = 1 + .9 * Math.sin(ph);
  const ss = x => { x = Math.max(0, Math.min(1, x)); return x * x * (3 - 2 * x); }, G = (d, w) => Math.exp(-(d * d) / (w * w)), ang = (v, d) => Math.acos(Math.max(-1, Math.min(1, v.dot(d))));
  const C0 = [0, .74, -.04], S0 = [.47, .6, .62], two = f => [-1, 1].map(f);
  const eyes = two(k => V(k * .4, .0, .92).normalize()), nose = V(0, -.3, .95).normalize(), cheeks = two(k => V(k * .74, -.2, .64).normalize()), temples = two(k => V(k * .92, .0, .38).normalize()), maxi = V(0, -.66, .75).normalize();
  // One piece of bone. The sockets and the nose are hollows in it, so their rims catch the light and their floors fall dark.
  const sock = (v, e, k) => { if (v.z < 0) return 1; const dx = (v.x - e.x) * k, dy = v.y - e.y + .3 * Math.max(0, dx) - .1 * Math.max(0, -dx); return ss(Math.hypot(dx / .36, dy / .27)); };
  const f = v => {
    let k = 1 - .3 * ss((-v.y - .15) / .6) * Math.abs(v.x);
    k *= 1 - .42 * ss((-v.y - .25) / .5) * ss((.35 - v.z) / .7);
    k += .05 * G(v.y - .2, .1) * ss((v.z - .3) / .4) * (1 - .5 * G(v.x, .1));
    k += .07 * (G(ang(v, cheeks[0]), .26) + G(ang(v, cheeks[1]), .26)) - .07 * (G(ang(v, temples[0]), .3) + G(ang(v, temples[1]), .3)) + .07 * G(ang(v, maxi), .3);
    k -= .3 * (2 - sock(v, eyes[0], -1) - sock(v, eyes[1], 1)) + .24 * (1 - ss(Math.hypot(v.x / .13, (v.y - nose.y) / .2))) * (v.z > 0 ? 1 : 0);
    return k + (fbm(n, v.x * 4, v.y * 4 + v.z * 3, 3) - .5) * .05;
  };
  const on = (v, lift) => { v = v.clone().normalize(); const k = f(v) + (lift || 0); return [C0[0] + v.x * S0[0] * k, C0[1] + v.y * S0[1] * k, C0[2] + v.z * S0[2] * k]; }, at = (d, k) => [C0[0] + d.x * S0[0] * k, C0[1] + d.y * S0[1] * k, C0[2] + d.z * S0[2] * k];
  const cran = blob(S0, f, bn, { p: C0, seg: 180, seg2: 110 });
  const arches = two(k => rod([on(cheeks[k < 0 ? 0 : 1], -.03), [k * .5, .6, .16], on(V(k * .97, -.2, -.1), -.03)], s => .03 + .012 * Math.sin(s * Math.PI), bn, { n: 16, sides: 8 }));
  const sockets = eyes.map(d => ball(.15, dark(), { s: [1.15, .85, 1], p: at(d, .6) })), nasal = [ball(.075, dark(), { s: [.8, 1.3, 1], p: at(nose, .7) })];
  const lights = eyes.flatMap(d => [ball(.06, ghost(c.detail, .12 * pw), { p: at(d, .8) }), ball(.009 + .006 * pw, glow(c.detail, .6 + pw * 1.3), { p: at(d, .84) })]);
  const teeth = many(new THREE.CapsuleGeometry(.024, .06, 4, 8), bone('#d9cba8', { gloss: .6 }), Array.from({ length: 13 }, (_, i) => { const a = (i - 6) / 6 * 1.2, q = on(V(Math.sin(a) * .42, -.8, .55 * Math.cos(a) + .12)); return { p: [q[0], q[1] - .03, q[2]], s: [1, i === 2 || i === 10 ? 1.9 : 1 + (i % 3) * .12, .8] }; }));
  const hornOf = k => M(tube(path([[k * .26, 1.14, 0], [k * .62, 1.3, -.04], [k * .9, 1.56, .04], [k * .9, 1.9, .22], [k * .76, 2.12, .4]], 40), s => (.14 * Math.pow(1 - s, .8) + .008) * (1 + .05 * Math.sin(s * 70)), 18), hornM);
  return item('Horned skull', [part('skull', 'body', [0, 0, 0], [cran, teeth].concat(sockets, nasal)), part('horns', 'trim', [0, .5, 0], [hornOf(1), hornOf(-1)]), part('what looks back', 'detail', [0, 0, .9], lights)], { stand: true, yaw: .28, el: 12, halo: .6 });
}

export function amulet(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .24 }), link = metal(c.trim, { kind: 'cast', rough: .24 }), st = gem(c.detail, { inner: .3, edges: .1, body: .36, lit: .16 }), m4 = new THREE.Matrix4();
  const plate = turned([[0, -.03], [.3, -.03], [.325, .0], [.345, .07], [.375, .075], [.395, .034], [.43, .03], [.5, .018], [.53, .03], [.56, .05], [.61, .05], [.635, .03], [.64, 0], [.63, -.05], [.58, -.07], [0, -.07]], gold, { sm: 90, seg: 96, r: [Math.PI / 2, 0, 0] });
  const meshes = [plate];
  for (let s = 0; s < 2; s++) { const pts = [], tw = 44, nn = tw * 10; for (let i = 0; i < nn; i++) { const a = i / nn * Math.PI * 2, b = i / nn * tw * Math.PI * 2 + s * Math.PI, r = .585 + Math.cos(b) * .017; pts.push(V(Math.cos(a) * r, Math.sin(a) * r, .058 + Math.sin(b) * .017)); } meshes.push(M(tube(pts, .017, 7, true), gold)); }
  meshes.push(many(new THREE.SphereGeometry(.03, 14, 10), gold, Array.from({ length: 34 }, (_, i) => { const a = i / 34 * Math.PI * 2; return { p: [Math.cos(a) * .465, Math.sin(a) * .465, .034] }; })));
  for (let i = 0; i < 6; i++) { const a = i / 6 * Math.PI * 2 + Math.PI / 6; meshes.push(rod([[.372, 0, .03], [.366, 0, .085], [.345, 0, .13], [.3, 0, .158], [.262, 0, .168]], s => .024 * (1 - s * .45), gold, { n: 14, sides: 10, r: [0, 0, a] })); meshes.push(ball(.0135, gold, { p: [.262, 0, .168], r: [0, 0, a] })); }
  meshes.push(torus(.07, .022, gold, { p: [0, .695, 0] }), torus(.075, .018, gold, { r: [0, Math.PI / 2, .25], p: [0, .81, 0] }));
  const stone = M(cutStone(.33, .12, .1), st, { p: [0, 0, .062] });
  const ch = chain([[-2.3, 2.5, 0], [-1.6, 2.05, 0], [-1.0, 1.75, 0], [-.5, 1.28, 0], [-.14, .98, 0], [0, .875, 0], [.16, .99, 0], [.46, 1.36, 0], [.98, 1.62, 0], [1.5, 2.1, 0], [2.2, 2.45, 0]], .036, link, { nofit: true });
  return item('Amulet', [part('setting', 'body', [0, 0, -.22], meshes), part('stone', 'detail', [0, 0, .42], [stone]), part('chain', 'trim', [0, .3, 0], [ch])], { yaw: .2, fit: new THREE.Box3(V(-.78, -.66, -.05), V(.78, 1.25, .1)) });
}
