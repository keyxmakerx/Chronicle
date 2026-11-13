// Plain goods and gear the set was missing: a potion, ore, cloth, herbs, boots, a torch, a bomb and cut gems.
import { THREE, V, M, X, Y, Z, part, item, turned, rod, ball, torus, cyl, rbox, slab, blob, sheet, many, aim, crystalGeo, cutStone, flame, lampGlow, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { tex } from '../util.js';
import { metal, wood, leather, cloth, rock, wax, paper, matte, gem, glass, liquid, glow, ghost, dark } from '../mats.js';

const C = hex => new THREE.Color(hex);

export function potion(c, t) {
  const gl = glass(c.body, { tint: .14 }), tt = t || 0, ph = tt * Math.PI * 2, pw = 1 + .35 * Math.sin(ph), r = rng(5), lv = .62;
  const draught = liquid(c.detail, { lit: .13 * pw, inner: 1.5 }), waxM = wax(c.trim, { lit: .03 }), cord = matte('#b8a47a', { rough: .9, bump: .8 }), ck = wood('#9a7048', { bump: .4 });
  const body = turned([[0, .05], [.24, .01], [.4, .03], [.53, .2], [.57, .44], [.5, .7], [.34, .88], [.19, .97], [.15, 1.05], [.15, 1.24], [.2, 1.28], [.2, 1.32], [.14, 1.33]], gl, { sm: 110, seg: 64 });
  const fill = turned([[0, .09], [.24, .05], [.38, .07], [.49, .22], [.53, .44], [.512, lv + .014], [.49, lv], [0, lv - .004]], draught, { sm: 0, seg: 56 });
  // What it gives off gathers in the air above the draught and rises into the neck.
  const motes = Array.from({ length: 12 }, () => { const a = r() * 6.28, h = (r() + tt) % 1, d = r() * .36 * (1 - h * .8); return ball(.008 + r() * .008, glow(c.detail, 2.4 * Math.min(1, h * 5) * (1 - h), { add: true }), { p: [Math.cos(a) * d, lv + .02 + h * .5, Math.sin(a) * d], seg: 8, seg2: 6 }); });
  const cork = cyl(.12, .135, .16, ck, { p: [0, 1.38, 0] });
  const seal = turned([[0, 1.475], [.1, 1.47], [.16, 1.445], [.19, 1.39], [.2, 1.31], [.198, 1.25], [.185, 1.225], [.17, 1.24]], waxM, { sm: 50, seg: 48 }), drips = [];
  for (let i = 0; i < 6; i++) { const a = r() * 6.28, len = .08 + r() * .26, x = Math.cos(a), z = Math.sin(a), y1 = 1.24 - len, rr = y1 > 1.05 ? .162 : .162 + (1.05 - y1) * .5; drips.push(rod([[x * .2, 1.27, z * .2], [x * .17, 1.2, z * .17], [x * .165, (1.2 + y1) / 2, z * .165], [x * rr, y1, z * rr]], s => .02 + .012 * s, waxM, { n: 12, sides: 8 }), ball(.032, waxM, { p: [x * rr, y1, z * rr], s: [1, 1.3, 1] })); }
  const tie = [torus(.165, .013, cord, { r: [Math.PI / 2, 0, 0], p: [0, 1.1, 0] }), rod([[.16, 1.1, .05], [.36, 1.0, .14], [.56, .5, .26], [.66, .08, .34], [.74, .02, .4]], .011, cord, { n: 30, sides: 6 })];
  const tag = rbox(.34, .012, .2, .004, paper('#bfae82'), { r: [0, .5, 0], p: [.9, .012, .46] });
  return item('Healing potion', [part('flask', 'body', [0, 0, 0], [body]), part('draught', 'detail', [0, 0, .9], [fill].concat(motes)), part('seal', 'trim', [0, .5, 0], [cork, seal].concat(drips)), part('tag', '', [.5, 0, .3], tie.concat([tag]))], { stand: true, yaw: .2, el: 14, lights: [{ p: [0, .85, 0], color: c.detail, power: .9 * pw }], halo: .7 });
}

export function ore(c) {
  const n = makeNoise(73, 32), r = rng(19), gold = metal(c.detail, { kind: 'cast', rough: .34 }), S = [.84, .62, .68], cuts = [[V(0, -1, 0), .6]];
  for (let i = 0; i < 13; i++) { const z = r() * 1.7 - .7, a = r() * 6.283, q = Math.sqrt(Math.max(0, 1 - z * z)); cuts.push([V(Math.cos(a) * q, z, Math.sin(a) * q), .7 + r() * .2]); }
  // Broken out of the rock face, so it is all flat breaks; the under side is the one it rests on.
  const lump = v => { let q = 1 + (fbm(n, v.x * 1.6 + 3, v.y * 1.6 + v.z * 1.4, 3) - .5) * .3; for (const [nn, cc] of cuts) { const a = v.dot(nn); if (a * q > cc) q = cc / a; } return q * (1 + (fbm(n, v.x * 9 + 3, v.y * 9 + v.z * 8, 3) - .5) * .05); };
  // A quartz vein runs through it with a thinner one branching off, and the gold sits in the quartz.
  const nv = V(.35, .5, .8).normalize(), nv2 = V(-.7, .6, .2).normalize();
  const inVein = d => Math.abs(d.dot(nv) + (fbm(n, d.x * 2.2 + 5, d.y * 2.2 + d.z * 2, 3) - .5) * .5) < .085 + .06 * (fbm(n, d.x * 4 + 1, d.y * 4 + d.z * 3, 2) - .4) || Math.abs(d.dot(nv2) - .2 + (fbm(n, d.x * 3 + 8, d.y * 3 + d.z * 2, 2) - .5) * .3) < .022;
  const goldAt = d => fbm(n, d.x * 6 + 2, d.y * 6 + d.z * 5, 3) > .5;
  const dir = (u, v) => { const th = v * Math.PI, a = u * Math.PI * 2; return V(-Math.cos(a) * Math.sin(th), Math.cos(th), Math.sin(a) * Math.sin(th)); };
  const skin = (mat, k, hole) => { mat.alphaMap = tex(1024, 512, (u, v) => hole(dir(u, v)) ? 0 : 1); mat.alphaTest = .5; mat.side = THREE.DoubleSide; return blob(S.map(q => q * k), lump, mat, { p: [0, .372, 0], seg: 150, seg2: 100 }); };
  const host = skin(rock(c.body, { bump: 2.6, rough: .92, flat: true }), 1, inVein), vein = skin(rock(c.trim, { bump: .9, rough: .38, flat: true }), .986, d => inVein(d) && goldAt(d)), metalIn = blob(S.map(q => q * .972), lump, gold, { p: [0, .372, 0], seg: 150, seg2: 100 });
  const qz = gem(c.trim, { vc: true, frost: .4, body: .6, pale: .7, lit: .1, inner: .5, edges: .15 }), quartz = [vein];
  for (let i = 0, tries = 0; i < 4 && tries < 4000; tries++) { const d = V(r() * 2 - 1, r() * 1.1 - .1, r() * 1.4 - .4).normalize(); if (!inVein(d)) continue; const k = lump(d) * .95, len = .07 + r() * .08, m = M(crystalGeo(.026 + r() * .018, len, len * .4, { seed: i + 9, hex: c.trim }), qz); m.geometry.applyQuaternion(aim(d.clone().add(V((r() - .5) * .5, (r() - .5) * .5, (r() - .5) * .5)))); m.geometry.translate(d.x * k * S[0], .372 + d.y * k * S[1], d.z * k * S[2]); quartz.push(m); i++; }
  const bits = Array.from({ length: 7 }, (_, i) => { const a = .2 + r() * 2.7, d = .86 + r() * .2, s = .04 + r() * .07; return M(new THREE.DodecahedronGeometry(s, 0), i % 4 === 3 ? gold : rock(c.body, { bump: 1.6, flat: true }), { r: [r() * 3, r() * 3, r() * 3], p: [Math.cos(a) * d, s * .6, Math.sin(a) * d * .7 + .2], s: [1.2, .7, 1] }); });
  return item('Lump of ore', [part('host rock', 'body', [0, 0, -.3], [host].concat(bits)), part('quartz', 'trim', [0, .5, 0], quartz), part('the metal in it', 'detail', [0, 0, .8], [metalIn])], { stand: true, yaw: .35, el: 22 });
}

export function bolt(c) {
  const wool = cloth(c.body, { bump: .7, sheen: .7 }), hem = cloth(c.trim, { sheen: 1, bump: .3 }), wd = wood(c.detail, { bump: .8 }), n = makeNoise(4, 16), L = 1.7, cy = .2;
  // Wound flat round a board, so in section it is an oblong with round corners.
  const sec = (a, k) => { const cx = Math.cos(a), sy = Math.sin(a), q = Math.pow(Math.pow(Math.abs(cx), 4) + Math.pow(Math.abs(sy), 4), -.25); return [cx * q * .44 * k, sy * q * .18 * k]; };
  const wrapS = (k, v0, v1, mat, nv) => sheet((u, v) => { const q = sec(u * Math.PI * 2, k), x = (v0 + (v1 - v0) * v - .5) * L; return V(x, cy + q[1] + (n(u * 9, x * 3) - .5) * .008, q[0]); }, 96, nv, mat, { uvx: 5, uvy: 6 * (v1 - v0) });
  const roll = wrapS(1, 0, 1, wool, 24), bands = [wrapS(1.012, .035, .075, hem, 2), wrapS(1.012, .925, .965, hem, 2)];
  const ends = []; for (const e of [-1, 1]) { for (let k = 1; k <= 7; k++) { const pts = []; for (let i = 0; i < 64; i++) { const q = sec(i / 64 * Math.PI * 2, k / 7.4); pts.push(V(e * (L / 2 - .004), cy + q[1], q[0])); } ends.push(M(tube(pts, .013, 6, true), wool)); } }
  const board = rbox(L + .2, .028, .56, .01, wd, { p: [0, cy, 0] });
  // The loose end comes off the top, falls over the front edge and lies on the table.
  const cv = new THREE.CatmullRomCurve3([[.1, cy + .185], [.36, cy + .15], [.47, .2], [.53, .06], [.7, .02], [.95, .03], [1.2, .014]].map(q => V(0, q[1], q[0]))), fall = (v0, v1, mat, lift, nv) => sheet((u, v) => { const vv = v0 + (v1 - v0) * v, q = cv.getPoint(vv), x = (u - .5) * L * (1 + .03 * vv); return V(x + .06 * Math.sin(vv * 4) * vv, q.y + lift + .022 * Math.sin(u * 15 + vv * 5) * Math.max(0, vv - .45) * 2, q.z + .05 * Math.sin(u * 9 + 1) * Math.max(0, vv - .5) * 2); }, 60, nv, mat, { uvx: 5, uvy: 4 * (v1 - v0) });
  const loose = fall(0, 1, wool, 0, 44), stripe = fall(.9, .975, hem, .004, 3);
  return item('Bolt of cloth', [part('cloth', 'body', [0, .2, 0], [roll, loose].concat(ends)), part('hem', 'trim', [0, .5, .5], bands.concat([stripe])), part('board', 'detail', [.9, 0, 0], [board])], { stand: true, yaw: .55, el: 30, fill: .9 });
}

export function herbs(c) {
  const r = rng(41), leafM = matte(c.body, { rough: .7, sheen: .3, bump: 1, double: true }), stemM = matte('#7a6a44', { rough: .8 }), flower = matte(c.detail, { rough: .6, sheen: .5 }), twine = matte(c.trim, { rough: .9, bump: .8 });
  const stems = [], leaves = [], buds = [];
  for (let i = 0; i < 28; i++) {
    const a = (r() - .5) * 1.15, lay = r(), top = V(Math.sin(a) * (1.0 + r() * .5), -.5 + Math.cos(a) * (1.25 + r() * .5), .04 + lay * .2), tie = V((r() - .5) * .16, -.5, .05 + lay * .15), end = V(tie.x * 1.7 + (r() - .5) * .1, -1.0 - r() * .22, .03 + lay * .1), P = path([end, tie, tie.clone().lerp(top, .5).add(V((r() - .5) * .1, 0, .04)), top], 30);
    stems.push(M(tube(P, s => .013 * (1 - s * .6), 6), stemM));
    if (i % 3 < 2) for (let j = 11; j <= 30; j += 2) { const tg = P[Math.min(30, j + 1)].clone().sub(P[j - 1]).normalize(), sd = j % 4 ? 1 : -1, d = tg.clone().applyAxisAngle(Z, sd * (.7 + r() * .4)).setZ((r() - .3) * .25).normalize(), len = (.1 + r() * .07) * (j === 30 ? 1.2 : 1); leaves.push({ p: P[j].clone().addScaledVector(d, len * .9), q: aim(d), s: [.034 + r() * .014, len, .007] }); }
    else for (let j = 17; j <= 30; j++) for (let k = 0; k < 3; k++) { const a2 = j * 2.4 + k * 2.1; buds.push({ p: [P[j].x + Math.cos(a2) * .022, P[j].y, P[j].z + Math.sin(a2) * .022 + .004], r: [0, 0, a], s: [.019, .03, .019] }); }
  }
  const leafG = new THREE.SphereGeometry(1, 10, 6), hel = [];
  for (let i = 0; i <= 150; i++) { const s = i / 150, a = s * Math.PI * 2 * 5; hel.push(V(Math.cos(a) * .15, -.56 + s * .12, .125 + Math.sin(a) * .115)); }
  const bind = [M(tube(hel, .014, 6), twine), rod([[.08, -.5, .24], [.3, -.4, .2], [.36, -.56, .12], [.14, -.54, .22]], .012, twine, { n: 20, sides: 6 }), rod([[.08, -.5, .24], [-.14, -.36, .2], [-.3, -.5, .1], [-.06, -.54, .22]], .012, twine, { n: 20, sides: 6 }), rod([[.08, -.5, .24], [.2, -.7, .1], [.3, -.86, .015]], .012, twine, { n: 12, sides: 6 }), ball(.03, twine, { p: [.08, -.5, .245] })];
  return item('Bundle of herbs', [part('stems', '', [0, -.3, 0], stems), part('leaves', 'body', [0, .3, .3], [many(leafG, leafM, leaves)]), part('twine', 'trim', [.5, -.2, .4], bind), part('flowers', 'detail', [0, .6, 0], [many(new THREE.SphereGeometry(1, 8, 6), flower, buds)])], { yaw: .15, fill: .92 });
}

export function boots(c) {
  const hide = leather(c.body, { bump: 1.3 }), dk = leather(c.detail, { bump: .9 }), br = metal(c.trim, { kind: 'cast', rough: .3 }), n = makeNoise(15, 16), Lf = .6, Rb = .22, La = Rb * Math.PI / 2, Ls = .9, T = Lf + La + Ls, N = 110, nA = 36, G = .05, sm = x => { x = Math.max(0, Math.min(1, x)); return x * x * (3 - 2 * x); };
  // The spine runs from the toe back to the heel, turns up at the ankle and climbs the leg.
  // Along the foot the upper is a half round sitting on the sole, so the toe is a low dome and not a tube end.
  const wide = x => { const u = Math.max(0, Math.min(1, (x + .45) / 1.08)), toe = u > .72 ? Math.sqrt(Math.max(0, 1 - Math.pow((u - .72) / .28, 2))) : 1, back = u < .2 ? Math.sqrt(Math.max(0, 1 - Math.pow((.2 - u) / .2, 2))) : 1; return (.16 + .03 * sm((u - .35) / .3)) * toe * back; };
  const topY = d => G + .15 * Math.sqrt(Math.max(0, 1 - Math.pow(1 - Math.min(1, d / .22), 2))) + .22 * sm((d - .16) / (Lf - .16)), cy = d => G + .17 * sm((d - .25) / (Lf - .25));
  const at = d => { if (d < Lf) return { c: V(.612 - d * 1.02, cy(d), 0), nr: V(0, 1, 0), th: 0 }; if (d < Lf + La) { const th = (d - Lf) / Rb; return { c: V(-Math.sin(th) * Rb, .22 + Rb - Math.cos(th) * Rb, 0), nr: V(Math.sin(th), Math.cos(th), 0), th: th }; } const u = d - Lf - La; return { c: V(-Rb + .03 * Math.sin(u * 3.2), .22 + Rb + u, 0), nr: V(1, 0, 0), th: Math.PI / 2 }; };
  const crease = d => 1 + .04 * Math.sin(d * 44) * sm((d - .5) / .12) * sm((1.3 - d) / .25);
  const ha = d => (d < Lf ? topY(d) - cy(d) : d < Lf + La ? .2 - .03 * sm((d - Lf) / La) + .015 * Math.sin((d - Lf) / La * Math.PI) : .17 + .06 * sm((d - Lf - La) / Ls)) * crease(d);
  const hb = d => (d < Lf ? wide(.612 - d * 1.02) - .008 : d < Lf + La ? .152 : .152 + .058 * sm((d - Lf - La) / Ls)) * crease(d);
  const ringAt = (d, k) => { const q = at(d), sq = Math.pow(Math.pow(Math.abs(Math.sin(q.th)), 3) + Math.pow(Math.abs(Math.cos(q.th)), 3), -1 / 3), pts = []; for (let j = 0; j < nA; j++) { const ph = j / nA * Math.PI * 2, lump = 1 + (n(d * 5 + Math.cos(ph) * 2, Math.sin(ph) * 2 + 3) - .5) * .04, cs = Math.cos(ph); pts.push([cs * ha(d) * k * lump * (cs < 0 ? 1 + (sq - 1) * -cs : 1), Math.sin(ph) * hb(d) * k * lump]); } return ring(q.c, q.nr, Z, pts).map(p => { p.y = Math.max(G, p.y); return p; }); };
  const rings = []; for (let i = 0; i <= N; i++) rings.push(ringAt(.0003 + i / N * (T - .0003), 1));
  const upper = M(loft(rings, { flip: true }), hide); upper.material.side = THREE.DoubleSide;
  const cuffR = []; for (let i = 0; i <= 12; i++) cuffR.push(ringAt(T - .25 + i / 12 * .26, 1.075 - .02 * Math.abs(i / 12 - .5) * 2));
  const cuff = M(loft(cuffR, { flip: true }), dk); cuff.material.side = THREE.DoubleSide;
  const top = at(T - .14), inside = M(new THREE.CircleGeometry(1, 28), dark(), { s: [ha(T - .14) * .97, hb(T - .14) * .97, 1], r: [Math.PI / 2, 0, 0], p: [top.c.x, top.c.y, 0] });
  const out = []; for (let i = 0; i <= 40; i++) { const x = .63 - i / 40 * 1.08; out.push([x, wide(x)]); } for (let i = 39; i > 0; i--) { const x = .63 - i / 40 * 1.08; out.push([x, -wide(x)]); }
  const sole = slab(out.map(q => [q[0] * 1.02 + .002, q[1] * 1.05]), .07, .012, dk); sole.geometry.rotateX(Math.PI / 2); sole.geometry.translate(0, .035, 0);
  const welt = M(tube(out.map(q => V(q[0], .072, q[1])), .012, 6, true), hide);
  const heel = slab(out.filter(q => q[0] < -.1).map(q => [q[0] * 1.02 + .002, q[1] * 1.05]), .05, .008, dk); heel.geometry.rotateX(Math.PI / 2); heel.geometry.translate(0, -.022, 0);
  const one = [[upper, 'b'], [cuff, 'd'], [inside, 'b'], [sole, 'd'], [welt, 'b'], [heel, 'd']];
  for (const d of [Lf + La + .06, T - .44]) { const sp = at(d); one.push([M(tube(ringAt(d, 1.03), .024, 8, true), dk), 'd'], [rbox(.1, .1, .028, .008, br, { p: [sp.c.x + .03, sp.c.y, hb(d) * 1.05] }), 't'], [rbox(.1, .014, .014, .004, br, { p: [sp.c.x + .03, sp.c.y, hb(d) * 1.05 + .016] }), 't']); }
  const seam = []; for (let i = 0; i <= 30; i++) { const d = Lf + La * .7 + i / 30 * (T - .27 - Lf - La * .7), q = ringAt(d, 1.004); seam.push(q[nA / 2]); }
  one.push([M(tube(seam, .008, 5), dk), 'd']);
  // A heeled boot rests on its heel and the ball of the foot, so the pair is built level and then let down at the toe.
  const P = { b: [], t: [], d: [] };
  for (const [m, k] of one) {
    const g0 = m.geometry; g0.translate(.1, .047, 0); g0.rotateZ(-.085); g0.translate(-.1, 0, 0); P[k].push(m);
    const g = (g0.index ? g0 : g0.toNonIndexed()).clone(); if (!g.index) g.setIndex(Array.from({ length: g.attributes.position.count }, (_, i) => i));
    g.scale(1, 1, -1); const ix = g.index.array; for (let i = 0; i < ix.length; i += 3) { const q = ix[i]; ix[i] = ix[i + 1]; ix[i + 1] = q; }
    g.rotateY(.45); g.translate(.16, 0, -.6); P[k].push(new THREE.Mesh(g, m.material));
  }
  return item('Boots', [part('uppers', 'body', [0, .2, 0], P.b), part('buckles', 'trim', [0, .3, .7], P.t), part('soles and cuffs', 'detail', [0, -.3, 0], P.d)], { stand: true, yaw: -.6, el: 20 });
}

export function torch(c, t) {
  const wd = wood(c.body, { bump: 1.1 }), iron = metal(c.trim, { kind: 'forged', rough: .6 }), rag = matte('#2b1d13', { rough: .92, bump: 1.6, grain: 'rock' }), tt = t || 0, r = rng(3), H = Math.PI / 2;
  // An iron tripod holds it upright: a collar the shaft wedges into and a cup for its foot.
  const stand = [torus(.082, .02, iron, { r: [H, 0, 0], p: [0, .66, 0] }), torus(.3, .014, iron, { r: [H, 0, 0], p: [0, .2, 0] }), turned([[0, .12], [.09, .12], [.105, .15], [.105, .26], [.085, .26], [.085, .16], [0, .16]], iron, { sm: 0, seg: 32 })];
  for (let i = 0; i < 3; i++) { const a = i * 2.094 + .5, x = Math.cos(a), z = Math.sin(a); stand.push(rod([[x * .1, .66, z * .1], [x * .13, .56, z * .13], [x * .3, .2, z * .3], [x * .43, .02, z * .43]], .022, iron, { n: 16, sides: 8 }), ball(.034, iron, { p: [x * .44, .024, z * .44], s: [1.4, .7, 1.4] }), rod([[x * .1, .2, z * .1], [x * .3, .2, z * .3]], .013, iron, { sides: 6 })); }
  const shaft = turned([[0, .16], [.05, .16], [.054, .6], [.064, 1.2], [.074, 1.45], [0, 1.45]], wd, { sm: 30, seg: 20 });
  // The head is rag wound round and round and soaked in pitch, so it shows the windings.
  const head = turned([[.07, 1.3], [.13, 1.36], [.175, 1.5], [.185, 1.66], [.16, 1.8], [.1, 1.9], [0, 1.93]], rag, { sm: 40, seg: 56 });
  { const q = head.geometry.attributes.position, nz = makeNoise(8, 16); for (let i = 0; i < q.count; i++) { const x = q.getX(i), y = q.getY(i), z = q.getZ(i), a = Math.atan2(z, x), k = 1 + .075 * Math.pow(.5 + .5 * Math.sin(y * 48 + a), 1.6) + (nz(a * 3 + 9, y * 9) - .5) * .09; q.setXYZ(i, x * k, y, z * k); } head.geometry.computeVertexNormals(); }
  const wire = [1.345, 1.375, 1.52].map((y, i) => torus(i < 2 ? .118 + i * .018 : .192, .008, iron, { r: [H, 0, .1 * i], p: [0, y, 0] }));
  const fl = []; for (const [h, rr, dt, x, z] of [[.8, .15, 0, 0, 0], [.56, .1, .37, .11, .03], [.5, .095, .71, -.1, -.04], [.44, .085, .2, .03, -.11], [.4, .08, .55, -.04, .11], [.3, .07, .85, .12, -.08]]) { const f = flame(h, rr, c.detail, (tt + dt) % 1, () => glow('#ffffff', .34, { vc: true, add: true })); f.forEach(m => m.geometry.translate(x, 1.76, z)); fl.push(...f); }
  fl.push(lampGlow(head, [0, 1.95, 0], c.detail, .22, .2), lampGlow(shaft, [0, 1.8, 0], c.detail, .12, .4));
  const embers = Array.from({ length: 14 }, () => { const a = r() * 6.28, d = r() * .16, h = (r() + tt) % 1, m = ball(.008 + r() * .008, glow(c.detail, 2.6 * (1 - h) * Math.min(1, h * 8), { add: true }), { p: [Math.cos(a) * d + .12 * Math.sin(h * 5 + a), 2.3 + h * 1.0, Math.sin(a) * d], seg: 8, seg2: 6 }); m.userData.nofit = true; return m; });
  return item('Torch', [part('stand', 'trim', [0, -.4, 0], stand.concat(wire)), part('shaft', 'body', [0, 0, 0], [shaft]), part('pitch', '', [0, .3, 0], [head]), part('flame', 'detail', [0, .7, 0], fl.concat(embers))], { stand: true, yaw: .3, el: 12, lights: [{ p: [0, 2.3, 0], color: c.detail, power: 3.6, flick: .22 }], halo: .9 });
}

export function bomb(c, t) {
  const tt = t || 0, ph = tt * Math.PI * 2, iron = metal(c.body, { kind: 'forged', rough: .8, bump: 1.3 }), cordM = matte(c.trim, { rough: .9, bump: .9 }), r = rng(27), R = .6, tilt = .38, dir = V(Math.sin(tilt), Math.cos(tilt), 0);
  const shellM = ball(R, iron, { p: [0, R + .05, 0], seg: 64, seg2: 44 }), seam = torus(R + .003, .011, iron, { r: [Math.PI / 2 + .25, 0, .3], p: [0, R + .05, 0], seg: 96 });
  const neck = turned([[0, 0], [.18, 0], [.19, .03], [.16, .06], [.13, .13], [.15, .15], [.15, .2], [.045, .2], [.045, .15], [0, .15]], iron, { sm: 0, seg: 40 }), base = V(0, R + .05, 0).addScaledVector(dir, R - .03);
  neck.geometry.rotateZ(-tilt); neck.geometry.translate(base.x, base.y, base.z);
  const f0 = base.clone().addScaledVector(dir, .16), fp = [f0, f0.clone().addScaledVector(dir, .2), V(.56, 1.62, .06), V(.7, 1.78, .16), V(.62, 1.96, .22)], fuse = rod(fp, .024, cordM, { n: 30, sides: 8 }), tip = fp[4];
  const grommet = torus(.3, .05, cordM, { r: [Math.PI / 2, 0, 0], p: [0, .05, 0], seg: 48 });
  // The fuse burns: a white-hot end, and sparks thrown out that arc and die.
  const hot = [ball(.03 + .008 * Math.sin(ph * 3), glow(c.detail, 3.2), { p: [tip.x, tip.y, tip.z] }), rod([fp[3].clone().lerp(tip, .55), tip], .026, glow(c.detail, .9), { sides: 8 })], sparks = [];
  for (let i = 0; i < 22; i++) { const d = V(r() - .5, r() * .9 - .1, r() - .5).normalize(), rate = 1 + Math.floor(r() * 3), u = (tt * rate + r()) % 1, far = .16 + r() * .3, pos = tip.clone().addScaledVector(d, u * far).add(V(0, -.22 * u * u, 0)), back = tip.clone().addScaledVector(d, Math.max(0, u - .14) * far).add(V(0, -.22 * Math.max(0, u - .14) * Math.max(0, u - .14), 0)), m = M(tube([back, pos], .006, 4), glow(c.detail, 3.2 * (1 - u), { add: true })); m.userData.nofit = true; sparks.push(m); }
  const smoke = [0, 1].map(i => { const pts = []; for (let j = 0; j <= 22; j++) { const s = j / 22; pts.push(V(tip.x + Math.sin(s * 5 + i * 2 - ph) * .08 * s + i * .03, tip.y + .04 + s * .7, tip.z + Math.cos(s * 4 + i - ph) * .06 * s)); } const m = rod(pts, s => .008 + .05 * s, ghost('#b8b0a8', .07), { sides: 8 }); m.userData.nofit = true; return m; });
  const warm = lampGlow(shellM, [tip.x, tip.y, tip.z], c.detail, .5, .3);
  return item('Bomb', [part('shell', 'body', [0, 0, 0], [shellM, seam, neck, warm]), part('fuse and rope', 'trim', [.4, .3, 0], [fuse, grommet]), part('the lit end', 'detail', [.3, .7, .5], hot.concat(sparks, smoke))], { stand: true, yaw: .3, el: 14, lights: [{ p: [tip.x, tip.y + .05, tip.z + .1], color: c.detail, power: 1.5, flick: .3, rate: 5 }], halo: .8 });
}

export function gems(c) {
  const r = rng(33), velvet = cloth(c.trim, { sheen: 1, bump: .25 }), n = makeNoise(6, 16), zc = (x, y) => { const u = x / 2.3 + .5, v = y / 1.9 + .5, e = Math.max(Math.abs(u - .5), Math.abs(v - .5)) * 2; return .015 + .06 * Math.max(0, fbm(n, u * 3, v * 3, 3) - .35) * (.3 + e) + .07 * Math.pow(e, 5); };
  const cl = sheet((u, v) => { const x = (u - .5) * 2.3, y = (v - .5) * 1.9; return V(x + .06 * Math.sin(v * 5) * Math.abs(u - .5), y + .05 * Math.sin(u * 4) * Math.abs(v - .5), zc(x, y)); }, 60, 50, velvet, { uvx: 4, uvy: 4 });
  const o = { inner: .45, edges: .22, body: .34, lit: .18, fire: 3.4 }, mats = [gem(c.body, o), gem(c.detail, o), gem('#eef2f8', { inner: .3, edges: .3, body: .16, lit: .1, fire: 4, pale: .5 })];
  // [x, y, size, which stone, stretch across, stretch along]: round, oval and long cuts.
  const list = [[-.1, .1, .3, 0, 1, 1], [.5, .3, .2, 0, 1, 1.5], [-.62, -.26, .17, 0, 1, 1], [.2, -.46, .15, 0, .8, 1.7], [.0, .56, .13, 0, 1, 1], [.68, -.24, .24, 1, 1, 1.25], [-.5, .46, .19, 1, 1, 1], [-.22, -.42, .13, 1, 1, 1], [.34, -.05, .16, 2, 1, 1], [-.82, .12, .11, 2, 1, 1], [.84, .5, .09, 1, 1, 1], [-.36, .0, .08, 2, 1, 1]];
  const sets = [[], [], []];
  for (const [x, y, R, k, sx, sy] of list) { const g = cutStone(R, R * .34, R * .66); g.scale(sx, sy, 1); g.rotateX((r() > .3 ? 1 : -1) * (.75 + r() * .3)); g.rotateZ(r() * 6.28); g.computeBoundingBox(); g.translate(x, y, zc(x, y) + .003 - g.boundingBox.min.z); sets[k].push(new THREE.Mesh(g, mats[k])); }
  return item('Cut gems', [part('cloth', 'trim', [0, 0, -.3], [cl]), part('stones', 'body', [-.3, 0, .4], sets[0]), part('more stones', 'detail', [.4, 0, .7], sets[1].concat(sets[2]))], { yaw: .2, fill: .9 });
}
