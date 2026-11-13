// Magic and alchemy.
import { THREE, V, M, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, slab, blob, sheet, fan, chain, many, band, drawn, crystalGeo, flame, lampGlow, path, tube, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, rock, wax, gem, glass, glow, ghost, dark, paper, cloth, matte, leather } from '../mats.js';

export function crystal(c) {
  const n = makeNoise(5, 32), rk = rock(c.trim, { bump: 2.4 }), g1 = gem(c.body, { vc: true }), g2 = gem(c.detail, { vc: true });
  const bed = blob([.8, .3, .62], v => 1 + (fbm(n, v.x * 2.6 + 3, v.z * 2.6 + v.y * 2, 5) - .5) * 1.0 + (fbm(n, v.x * 9 + 1, v.z * 9 + v.y * 7, 3) - .5) * .1, rk, { p: [0, .13, 0], seg: 120, seg2: 80 });
  const grow = (list, mat, hex) => list.map(s => M(crystalGeo(s[2], s[3], s[2] * 1.9, { seed: s[6], hex: hex }), mat, { r: [s[4], 0, s[5]], p: [s[0], .15, s[1]] }));
  const big = grow([[0, 0, .17, 1.5, .04, -.06, 1], [-.3, .05, .13, 1.05, .1, .34, 2], [.3, -.04, .12, .92, -.08, -.42, 3], [.08, .26, .1, .62, .5, -.1, 4], [-.14, -.24, .09, .7, -.42, .16, 5]], g1, c.body);
  const young = grow([[-.5, .2, .06, .36, .3, .7, 6], [.5, .18, .055, .32, .4, -.8, 7], [.34, .34, .05, .3, .7, -.4, 8], [-.36, -.3, .05, .34, -.6, .5, 9], [.14, -.36, .045, .26, -.8, -.2, 10], [-.1, .42, .05, .28, .9, .1, 11]], g2, c.detail);
  return item('Crystal cluster', [part('bedrock', 'trim', [0, -.45, 0], [bed]), part('shards', 'body', [0, .3, 0], big), part('young growth', 'detail', [0, 0, .7], young)], { stand: true, yaw: .35, el: 17 });
}

export function candle(c, t) {
  const br = metal(c.trim, { kind: 'cast', rough: .42 }), wx = wax('#' + new THREE.Color(c.body).multiplyScalar(.72).getHexString(), { lit: .04 }), r = rng(4);
  const dish = turned([[0, .02], [.3, .02], [.5, .03], [.64, .08], [.69, .17], [.66, .18], [.6, .11], [.48, .07], [.25, .06], [0, .06]], br, { sm: 80 });
  const foot = turned([[0, 0], [.26, 0], [.28, .02], [.24, .04], [0, .04]], br);
  const loop = torus(.13, .032, br, { p: [.8, .13, 0] });
  const nz = makeNoise(31, 32);
  const pillar = turned([[0, .06], [.2, .06], [.215, .1], [.205, .5], [.2, .96], [.196, 1.03], [.175, 1.05], [.15, 1.035], [.1, .99], [.04, .975], [0, .975]], wx, { sm: 110, seg: 72 });
  { const q = pillar.geometry.attributes.position; for (let i = 0; i < q.count; i++) { const x = q.getX(i), y = q.getY(i), z = q.getZ(i), a = Math.atan2(z, x), k = 1 + (fbm(nz, a * 2.2 + 5, y * 3, 3) - .5) * .09 + (y > .9 ? (fbm(nz, a * 3 + 9, 2, 2) - .5) * .12 * (y - .9) / .15 : 0); q.setXYZ(i, x * k, y + (y > .95 ? (nz(a * 2 + 20, 1) - .5) * .05 : 0), z * k); } pillar.geometry.computeVertexNormals(); }
  const drips = [];
  for (let i = 0; i < 7; i++) { const a = r() * Math.PI * 2, len = .18 + r() * .6, x = Math.cos(a) * .205, z = Math.sin(a) * .205, y1 = 1.03 - len; drips.push(rod([[x * .9, 1.04, z * .9], [x * 1.03, 1.0, z * 1.03], [x * 1.06, (1.03 + y1) / 2, z * 1.06], [x * 1.07, y1, z * 1.07]], s => .02 + .016 * s * s, wx, { n: 14, sides: 8 })); drips.push(ball(.036, wx, { p: [x * 1.07, y1, z * 1.07], s: [1, 1.3, 1] })); }
  const pool = blob([.36, .02, .34], v => 1 + (fbm(nz, v.x * 2 + 2, v.z * 2, 2) - .5) * .5, wx, { p: [.02, .075, 0] });
  const wick = rod([[0, .97, 0], [.004, 1.05, 0], [.02, 1.1, 0]], .008, dark(), { n: 8, sides: 6 });
  const fl = flame(.36, .075, c.detail, t, () => glow('#ffffff', 1, { vc: true, add: true })); fl.forEach(m => m.geometry.translate(.012, 1.07, 0)); fl.push(lampGlow(pillar, [0, 1.12, 0], c.detail, .16, .16));
  return item('Ritual candle', [part('dish', 'trim', [0, -.4, 0], [dish, foot, loop]), part('wax', 'body', [0, 0, 0], [pillar, pool].concat(drips)), part('flame', 'detail', [0, .45, 0], [wick].concat(fl))], { stand: true, yaw: -.5, el: 14, lights: [{ p: [0, 1.3, 0], color: c.detail, power: 2.6, flick: .22 }], halo: .9 });
}

export function orb(c, t) {
  const wd = wood('#3a2a20', { polish: .4 }), mt = metal(c.trim, { kind: 'cast', rough: .25 }), gl = glass(c.body, { tint: .5 }), ph = (t || 0) * Math.PI * 2;
  const base = turned([[0, 0], [.5, 0], [.54, .03], [.52, .08], [.42, .1], [.34, .16], [.3, .26], [.34, .34], [.3, .38], [0, .38]], wd, { sm: 90 });
  const cup = torus(.3, .035, mt, { r: [Math.PI / 2, 0, 0], p: [0, .42, 0] });
  const claws = []; for (let i = 0; i < 3; i++) { const a = i / 3 * Math.PI * 2 + .5; claws.push(rod([[.28, .4, 0], [.44, .55, 0], [.52, .8, 0], [.47, 1.02, 0], [.4, 1.1, 0]], s => .034 * (1 - s * .55), mt, { n: 18, r: [0, a, 0] })); }
  const R = .5, cy = .9, sphere = ball(R, gl, { p: [0, cy, 0], seg: 64, seg2: 44 });
  const vis = [], gm = () => ghost(c.detail, .16, { power: 1 });
  for (let i = 0; i < 5; i++) { const pts = []; for (let j = 0; j <= 40; j++) { const s = j / 40, a = s * 5 + i * 1.26 + ph, rr = R * .72 * Math.sin(s * Math.PI) * (.5 + .12 * i); pts.push(V(Math.cos(a) * rr, cy + (s - .5) * R * 1.1 * Math.cos(i), Math.sin(a) * rr)); } const m = rod(pts, s => .006 + .022 * Math.sin(s * Math.PI), gm(), { sides: 8 }); m.renderOrder = 1; vis.push(m); }
  const core = ball(.055, glow(c.detail, 1.6, { add: true }), { p: [0, cy, 0] }); core.renderOrder = 1; vis.push(core);
  const haze = ball(R * .8, ghost(c.detail, .05), { p: [0, cy, 0] }); haze.renderOrder = 1; vis.push(haze);
  return item('Seeing orb', [part('base', 'trim', [0, -.4, 0], [base]), part('cradle', 'trim', [0, -.12, 0], [cup].concat(claws)), part('orb', 'body', [0, .3, 0], [sphere]), part('what it shows', 'detail', [0, .3, .9], vis)], { stand: true, yaw: .4, el: 15, lights: [{ p: [0, cy, 0], color: c.detail, power: 1.2 }], halo: .6 });
}

export function scroll(c) {
  const br = metal(c.body, { kind: 'cast' }), wd = wood(c.trim, { polish: .4 }), silk = cloth(c.detail, { sheen: 1, bump: .2 }), waxM = matte(c.detail, { rough: .35, wet: .5 });
  const page = drawn(512, 1024, (x, w, h) => { const r = rng(12); x.strokeStyle = 'rgba(52,34,20,.82)'; x.lineWidth = 2.2; x.lineCap = 'round';
    for (let l = 0; l < 17; l++) { const y = 250 + l * 31; if (l === 8) continue; let xx = 70; while (xx < w - 80) { const len = 14 + r() * 46; x.beginPath(); x.moveTo(xx, y + (r() - .5) * 3); for (let k = 0; k < len; k += 6) x.lineTo(xx + k, y + (r() - .5) * 9); x.stroke(); xx += len + 9 + r() * 8; } }
    x.lineWidth = 3; x.beginPath(); x.arc(w / 2, 150, 46, 0, 7); x.moveTo(w / 2, 104); x.lineTo(w / 2 - 40, 174); x.lineTo(w / 2 + 40, 174); x.closePath(); x.stroke(); });
  const R = .1, prof = v => { const L = 1.25, s = (v - .5) * 2, a = Math.abs(s), sg = Math.sign(s); if (a < .56) return [sg * a / .56 * L * .5, .012 + .02 * Math.pow(a / .56, 2)]; const t = (a - .56) / .44, th = t * Math.PI * 2 * 1.6, rr = R * (1 - t * .32); return [sg * (L * .5 + Math.sin(th) * rr), .012 + .02 + R - Math.cos(th) * rr]; };
  const sheetM = sheet((u, v) => { const q = prof(v); return V((u - .5) * 1.1, q[0], q[1]); }, 6, 180, paper('#ffffff', { map: page }));
  const roller = y => [cyl(.035, .035, 1.36, wd, { r: [0, 0, Math.PI / 2], p: [0, y, .012 + .02 + .1] })];
  const knobs = [-1, 1].flatMap(k => [-1, 1].map(e => turned([[.036, 0], [.07, .02], [.08, .07], [.05, .11], [.03, .14], [.045, .17], [0, .2]], br, { sm: 30, r: [0, 0, -e * Math.PI / 2], p: [e * .68, k * .625, .132] })));
  const seal = blob([.14, .14, .035], v => 1 + .08 * Math.cos(Math.atan2(v.y, v.x) * 7), waxM, { p: [.3, -.36, .04] }), stamp = torus(.07, .012, waxM, { p: [.3, -.36, .07] });
  const rib = sheet((u, v) => V(.3 + (u - .5) * .09 + v * .3, -.4 - v * .75, .02 + .03 * Math.sin(v * 8)), 2, 30, silk), rib2 = sheet((u, v) => V(.3 + (u - .5) * .09 - v * .1, -.4 - v * .8, .016 + .03 * Math.sin(v * 7 + 1)), 2, 30, silk);
  return item('Scroll', [part('page', '', [0, 0, 0], [sheetM]), part('rollers', 'trim', [0, 0, .4], roller(.625).concat(roller(-.625))), part('caps', 'body', [.5, 0, .3], knobs), part('ribbon', 'detail', [.3, -.4, .1], [rib, rib2]), part('seal', 'detail', [.3, -.2, .6], [seal, stamp])], { yaw: -.25, fill: .88 });
}

export function runestone(c, t) {
  const rk = rock(c.body, { bump: 2, rough: .9 }), cord = matte(c.trim, { rough: .85, bump: .8 }), n = makeNoise(17, 32), ph = (t || 0) * Math.PI * 2, pw = 1.9 * (1 + .55 * Math.sin(ph));
  const out = [[0, 1.0], [.3, .96], [.5, .7], [.56, .2], [.6, -.4], [.5, -.86], [.2, -1.0], [-.24, -.98], [-.52, -.8], [-.58, -.3], [-.54, .3], [-.44, .76], [-.26, .96]];
  const zf = (x, y, v) => (.2 * Math.pow(Math.max(0, 1 - Math.pow(v == null ? 0 : v, 4)), .5) + (n(x * 2.4 + 3, y * 2.4) - .5) * .09 * (1 - Math.pow(v == null ? 0 : v, 3)));
  const zAt = (x, y) => .2 + (n(x * 2.4 + 3, y * 2.4) - .5) * .09;
  const front = fan(out, zf, rk, { nu: 120, nv: 30, uvx: 2, uvy: 2 }), back = fan(out, (x, y, v) => -zf(x + 5, y, v), rock(c.body, { bump: 2 }), { nu: 80, nv: 12 }); back.material.side = THREE.DoubleSide;
  const stroke = pts => rod(pts.map(q => [q[0], q[1], zAt(q[0], q[1]) + .004]), .024, glow(c.detail, pw), { n: 30, sides: 8 });
  const rune = [stroke([[-.14, -.58], [-.14, .0], [-.14, .56]]), stroke([[-.14, .56], [.2, .32], [-.14, .06]]), stroke([[-.14, .06], [.24, -.58]]), stroke([[-.3, -.2], [.02, -.34]])];
  const loop = []; for (let i = 0; i < 40; i++) { const a = i / 40 * Math.PI * 2; loop.push(V(Math.cos(a) * .47, .74 + Math.sin(a) * .03, Math.sin(a) * .23)); }
  const tie = [M(tube(loop, .028, 8, true), cord), rod([[.44, .72, .1], [.56, .5, .2], [.62, .1, .16]], .026, cord, { n: 14 }), rod([[.44, .7, .12], [.5, .5, .24], [.5, .2, .26]], .026, cord, { n: 14 }), ball(.05, cord, { p: [.45, .72, .12] })];
  return item('Runestone', [part('stone', 'body', [0, 0, 0], [front, back]), part('rune', 'detail', [0, 0, .7], rune), part('cord', 'trim', [0, .5, 0], tie)], { stand: true, yaw: .3, el: 12, lights: [{ p: [0, 0, .7], color: c.detail, power: 1.4 }], halo: .9 });
}

export function hourglass(c) {
  const gl = glass(c.body, { tint: .12 }), wd = wood(c.trim, { polish: .4 }), sand = matte(c.detail, { rough: .95, bump: .8 }), br = metal('#b98a3a', { kind: 'cast' });
  const plate = y => turned([[0, y], [.5, y], [.53, y + .02], [.53, y + .08], [.5, y + .1], [.46, y + .12], [0, y + .12]], wd, { sm: 0, seg: 56 });
  const bulb = turned([[0, .125], [.28, .14], [.4, .32], [.38, .54], [.2, .7], [.045, .8], [.2, .9], [.38, 1.06], [.4, 1.28], [.28, 1.46], [0, 1.475]], gl, { sm: 120, seg: 64 });
  const heap = turned([[0, .44], [.1, .38], [.25, .26], [.36, .2], [.33, .15], [0, .14]], sand, { sm: 40 }), top = turned([[0, .86], [.03, .83], [.1, .86], [.22, .93], [.33, 1.04], [.2, 1.02], [.06, .98], [0, .97]], sand, { sm: 40 }), fall = cyl(.012, .012, .42, sand, { p: [0, .63, 0] });
  const posts = []; for (let i = 0; i < 3; i++) { const a = i / 3 * Math.PI * 2 + .5; posts.push(turned([[.03, .12], [.045, .2], [.03, .3], [.026, .8], [.03, 1.3], [.045, 1.4], [.03, 1.48]], wd, { sm: 40, seg: 16, p: [Math.cos(a) * .45, 0, Math.sin(a) * .45] })); }
  const studs = many(new THREE.SphereGeometry(.022, 8, 6), br, [0, 1, 2].flatMap(i => { const a = i / 3 * Math.PI * 2 + .5; return [{ p: [Math.cos(a) * .45, 1.6, Math.sin(a) * .45] }]; }));
  return item('Hourglass', [part('plates', 'trim', [0, -.3, 0], [plate(0), plate(1.48), studs]), part('posts', 'trim', [.6, 0, 0], posts), part('glass', 'body', [0, 0, 0], [bulb]), part('sand', 'detail', [0, 0, .9], [heap, top, fall])], { stand: true, yaw: .2, el: 12 });
}

export function censer(c, t) {
  const br = metal(c.body, { kind: 'cast', rough: .3 }), gilt = metal(c.trim, { kind: 'cast', rough: .22 }), ph = (t || 0) * Math.PI * 2, n = makeNoise(41, 16), heat = 1 + .25 * Math.sin(ph * 2);
  const bowl = turned([[0, 0], [.3, 0], [.33, .03], [.3, .07], [.13, .12], [.11, .2], [.2, .27], [.42, .38], [.52, .56], [.53, .72], [.5, .74], [.47, .72], [.46, .58], [.36, .44], [0, .4]], br, { sm: 120, seg: 64 });
  const lid = turned([[.52, .78], [.53, .82], [.46, 1.0], [.3, 1.16], [.14, 1.24], [.1, 1.3], [.13, 1.36], [.08, 1.42], [.03, 1.5], [0, 1.52]], gilt, { sm: 90, seg: 64 });
  const holes = []; for (let k = 0; k < 2; k++) for (let i = 0; i < 9; i++) { const a = (i + k * .5) / 9 * Math.PI * 2, rr = k ? .36 : .47, y = k ? 1.1 : .96; holes.push({ p: [Math.cos(a) * rr, y, Math.sin(a) * rr], r: [0, -a + Math.PI / 2, 0], s: [1, 1.3, .5] }); }
  const vents = many(new THREE.SphereGeometry(.036, 10, 8), glow(c.detail, 1.5 * heat), holes);
  const ember = turned([[0, .74], [.45, .74], [.47, .77], [0, .78]], glow(c.detail, 1.2 * heat), { sm: 0, seg: 40 });
  const top = V(0, 2.5, 0), chains = [0, 1, 2].map(i => { const a = i / 3 * Math.PI * 2 + .4; return chain([[Math.cos(a) * .53, .7, Math.sin(a) * .53], [Math.cos(a) * .46, 1.5, Math.sin(a) * .46], [Math.cos(a) * .1, 2.36, Math.sin(a) * .1]], .028, gilt, { seed: i + 2 }); });
  const lugs = many(new THREE.TorusGeometry(.04, .012, 8, 16), gilt, [0, 1, 2].map(i => { const a = i / 3 * Math.PI * 2 + .4; return { p: [Math.cos(a) * .55, .68, Math.sin(a) * .55], r: [0, -a, 0] }; }));
  const hang = torus(.09, .022, gilt, { p: [0, 2.46, 0] });
  const smoke = [0, 1, 2].map(i => { const pts = []; for (let j = 0; j <= 30; j++) { const s = j / 30; pts.push(V(Math.sin(s * 5 + i * 2 + ph) * .1 * (.3 + s) + (i - 1) * .06, 1.5 + s * 1.1, Math.cos(s * 4 + i + ph) * .08 * (.3 + s))); } return rod(pts, s => .012 + .05 * s, ghost('#b8b0a8', .1 * (1 - 0)), { sides: 8 }); });
  return item('Censer', [part('bowl', 'body', [0, -.2, 0], [bowl]), part('embers', 'detail', [0, .1, .8], [ember, vents]), part('lid', 'trim', [0, .5, 0], [lid]), part('chains', 'trim', [.5, .6, 0], chains.concat([lugs, hang])), part('smoke', '', [0, .9, 0], smoke)], { stand: true, yaw: .2, el: 8, lights: [{ p: [0, .86, 0], color: c.detail, power: 1.5, flick: .2 }], halo: .8 });
}
