// More things worn: a buckler, a chain shirt, a leather jerkin, a robe, a belt, bracers, a backpack, a brooch, a bracelet, spectacles and a hood.
import { THREE, V, M, X, Y, Z, part, item, turned, rod, ball, torus, cyl, rbox, slab, shape, hole, blob, sheet, many, aim, band, wrap, strands, chain, cutStone, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, matte, gem, glass, dark } from '../mats.js';
import { tex } from '../util.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
// Cloth on a lump: the weave drawn n times finer, on this material's own copy of the textures.
const fine = (m, n) => { for (const k of ['map', 'bumpMap']) if (m[k]) { m[k] = m[k].clone(); m[k].repeat.set(n, n); m[k].needsUpdate = true; } return m; };
const sstep = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
// A hanging garment: cloth round a body. u goes round it, v from the neck down to the hem.
// w(v) gives the half-widths [side to side, front to back], y(v) the height, fold(a, v) the
// folds; gap leaves the front open by that angle each side, and out lifts a band off the cloth.
const garment = o => (u, v, out) => {
  const a = o.gap ? o.gap + u * (TAU - 2 * o.gap) : Math.PI + u * TAU, w = o.w(v), f = (o.fold ? o.fold(a, v) : 0) + (out || 0);
  return V(Math.sin(a) * (w[0] + f), o.y(v, a), Math.cos(a) * (w[1] + f));
};
// A sleeve: a tube of cloth from the shoulder out and down, r(v) wide.
const sleeve = (k, from, to, r, sag) => (u, v) => {
  const a = u * TAU, d = V(to[0] - from[0], to[1] - from[1], 0).normalize(), nrm = V(-d.y, d.x, 0), rr = r(v);
  return V(k * (from[0] + (to[0] - from[0]) * v) + k * nrm.x * Math.cos(a) * rr, from[1] + (to[1] - from[1]) * v + nrm.y * Math.cos(a) * rr - (sag || 0) * Math.sin(v * Math.PI) * (1 - Math.cos(a)) * .5, Math.sin(a) * rr * .85);
};

export function buckler(c) {
  const face = metal(c.body, { kind: 'forged', rough: .42 }), br = metal(c.trim, { kind: 'cast' }), hide = leather(c.detail), R = .72;
  const dome = r => .1 * (1 - Math.pow(r / R, 2)), prof = [];
  for (let i = 0; i <= 24; i++) { const r = i / 24 * R; prof.push([r, dome(r)]); } for (let i = 24; i >= 0; i--) { const r = i / 24 * R; prof.push([r, dome(r) - .04]); }
  const disc = turned(prof, face, { sm: 0, seg: 72, r: [H2, 0, 0] });
  const boss = turned([[0, .32], [.06, .31], [.14, .26], [.2, .16], [.215, .08], [.26, .07], [.3, .055], [.3, .04]], br, { sm: 40, seg: 40, r: [H2, 0, 0], p: [0, 0, .04] });
  const nub = turned([[0, .08], [.02, .07], [.03, .03], [.045, 0]], br, { sm: 16, seg: 16, r: [H2, 0, 0], p: [0, 0, .35] });
  const edge = torus(R, .034, br), inner = torus(.5, .014, br, { p: [0, 0, dome(.5) + .004] });
  const rivets = many(new THREE.SphereGeometry(.028, 12, 8), br, Array.from({ length: 12 }, (_, i) => { const a = i / 12 * TAU; return { p: [Math.cos(a) * .62, Math.sin(a) * .62, dome(.62) + .008], s: [1, 1, .6] }; }));
  // The carrying thong, looped through two of the rivets.
  const thong = rod([[-.31, .54, .05], [-.36, .86, .03], [0, 1.08, .01], [.36, .86, .03], [.31, .54, .05]], .017, hide, { n: 30, sides: 7 });
  const washers = [-1, 1].map(k => torus(.036, .014, hide, { p: [.31 * k, .54, dome(.62) + .012] }));
  return item('Buckler', [part('face', 'body', [0, 0, 0], [disc]), part('boss and rim', 'trim', [0, 0, .6], [boss, nub, edge, inner, rivets]), part('thong', 'detail', [0, .5, .2], [thong].concat(washers))], { yaw: .5, fill: .9 });
}

export function chainshirt(c) {
  // Rings in rows, each row half a ring along from the last.
  const rings = tex(256, 256, (u, v) => { const row = Math.floor(v * 16), fx = ((u * 16 + (row % 2) * .5) % 1) - .5, fy = ((v * 16) % 1) - .5, d = Math.hypot(fx, fy * 1.2); return .16 + .84 * Math.exp(-Math.pow((d - .33) / .12, 2)); });
  const mailOf = hex => { const m = metal(hex, { kind: 'forged', rough: .34, double: true }); m.map = rings; m.bumpMap = rings; m.bumpScale = 2.4; return m; };
  const mail = mailOf(c.body), brass = mailOf(c.trim), hide = leather(c.detail, { double: true }), n = makeNoise(31, 16);
  const g = garment({
    w: v => { const sh = Math.sin(Math.min(1, v / .1) * H2), waist = Math.sin(sstep(.1, .7, v) * Math.PI); return [.2 + .32 * sh - .05 * waist + .05 * sstep(.7, 1, v), .15 + .13 * sh - .02 * waist + .03 * sstep(.7, 1, v)]; },
    y: v => 1.52 - .13 * (1 - Math.cos(Math.min(1, v / .1) * H2)) - Math.max(0, v - .1) / .9 * 1.78,
    fold: (a, v) => Math.sin(a * 6 + n(a, 2) * 3) * (.006 + .03 * sstep(.35, 1, v)) + (n(a * 2, v * 5) - .5) * .02
  });
  const body = sheet((u, v) => g(u, v, 0), 160, 60, mail, { uvx: 4, uvy: 3 });
  const hem = sheet((u, v) => g(u, .93 + v * .07, .008), 160, 4, brass, { uvx: 4, uvy: .25 });
  const arm = k => sleeve(k, [.44, 1.36], [.9, 1.02], v => .2 - .02 * v, .04);
  const sleeves = [-1, 1].map(k => sheet(arm(k), 48, 16, mail, { uvx: 1.6, uvy: .9 }));
  const cuffs = [-1, 1].map(k => sheet((u, v) => { const p = arm(k)(u, .86 + v * .14), q = arm(k)(u + .5, .86 + v * .14); return p.add(p.clone().sub(q).multiplyScalar(.02)); }, 48, 3, brass, { uvx: 1.6, uvy: .2 }));
  // A hide yoke at the neck, laced at the throat.
  const yoke = sheet((u, v) => g(u, .005 + v * .075, .012), 80, 6, hide);
  const lace = rod([[-.04, 1.47, .2], [.04, 1.43, .23], [-.04, 1.39, .26], [.04, 1.35, .28], [.1, 1.2, .3]], .009, hide, { n: 20, sides: 5 }), lace2 = rod([[-.04, 1.35, .28], [-.12, 1.22, .3]], .009, hide, { sides: 5 });
  return item('Chainmail', [part('mail', 'body', [0, 0, 0], [body].concat(sleeves)), part('edging', 'trim', [0, -.4, .3], [hem].concat(cuffs)), part('yoke', 'detail', [0, .4, .3], [yoke, lace, lace2])], { stand: true, yaw: .3, el: 10, fill: .86 });
}

export function jerkin(c) {
  const hide = leather(c.body, { double: true, bump: 1.3 }), br = metal(c.trim, { kind: 'cast' }), cord = leather(c.detail), n = makeNoise(17, 16);
  // Sleeveless and open down the front, where it laces.
  const o = {
    gap: .14,
    w: v => { const sh = Math.sin(Math.min(1, v / .1) * H2), waist = Math.sin(sstep(.1, .85, v) * Math.PI); return [.2 + .3 * sh - .06 * waist + .06 * sstep(.75, 1, v), .15 + .14 * sh - .02 * waist + .03 * sstep(.75, 1, v)]; },
    y: v => 1.4 - .12 * (1 - Math.cos(Math.min(1, v / .1) * H2)) - Math.max(0, v - .1) / .9 * 1.36,
    fold: (a, v) => (n(a * 1.5, v * 4) - .5) * .03 + Math.sin(a * 4) * .012 * v
  }, g = garment(o);
  const body = sheet((u, v) => g(u, v, 0), 140, 50, hide, { uvx: 4, uvy: 3 });
  const edge = [0, 1].map(k => sheet((u, v) => g(k ? 1 - u * .02 : u * .02, v, .01), 2, 50, cord));
  const hemB = sheet((u, v) => g(u, .95 + v * .05, .01), 140, 3, cord);
  // Shoulder wings: a stiff cap over each arm hole.
  const wings = [-1, 1].map(k => sheet((u, v) => { const a = (u - .5) * 2.4; return V(k * (.46 + .1 * v * Math.cos(a * .6)), 1.31 - .16 * v * v - .05 * Math.abs(a), Math.sin(a) * (.2 + .03 * v)); }, 24, 8, hide));
  // Eyelets down both edges, and a lace crossed between them.
  const ys = Array.from({ length: 7 }, (_, i) => 1.16 - i * .17), at = v => { const p = g(.012, (1.4 - .12 - v) / 1.36 * .9 + .1, .014); return p; };
  const eye = ys.map(y => [-1, 1].map(k => { const p = at(y); return torus(.022, .009, br, { p: [k * Math.abs(p.x), p.y, p.z + .004] }); })).flat();
  const lp = []; ys.forEach((y, i) => { const p = at(y), k = i % 2 ? 1 : -1; lp.push(V(k * Math.abs(p.x), p.y, p.z + .016)); lp.push(V(-k * Math.abs(p.x), p.y - .085, p.z + .02)); });
  const lace = rod(lp, .008, cord, { sides: 5 }), lace2 = rod(lp.map(p => V(-p.x, p.y, p.z + .004)), .008, cord, { sides: 5 });
  const studs = many(new THREE.SphereGeometry(.02, 10, 8), br, Array.from({ length: 22 }, (_, i) => { const u = .08 + i / 21 * .84, p = g(u, .9, .012); return { p: [p.x, p.y, p.z] }; }));
  return item('Leather jerkin', [part('jerkin', 'body', [0, 0, 0], [body].concat(wings)), part('eyelets and studs', 'trim', [0, -.2, .5], eye.concat([studs])), part('lacing and edges', 'detail', [0, .2, .6], [lace, lace2, hemB].concat(edge))], { stand: true, yaw: .25, el: 10, fill: .86 });
}

export function robe(c) {
  const wool = cloth(c.body, { bump: .8, sheen: .5 }), band = cloth(c.trim, { sheen: 1 }), sash = cloth(c.detail, { sheen: .9, bump: .4 }), n = makeNoise(41, 32), r = rng(5);
  const o = {
    w: v => { const sh = Math.sin(Math.min(1, v / .07) * H2), waist = Math.exp(-Math.pow((v - .34) / .07, 2)); return [.18 + .28 * sh + .36 * sstep(.36, 1, v) - .07 * waist, .14 + .12 * sh + .3 * sstep(.36, 1, v) - .03 * waist]; },
    y: v => 2.1 - .12 * (1 - Math.cos(Math.min(1, v / .07) * H2)) - Math.max(0, v - .07) / .93 * 2.5,
    fold: (a, v) => Math.sin(a * 8 + n(a, 3) * 4) * (.004 + .09 * sstep(.38, 1, v)) + (n(a * 3, v * 4) - .5) * .03
  }, g = garment(o);
  const body = sheet((u, v) => g(u, v, 0), 220, 70, wool, { uvx: 9, uvy: 8 });
  const hem = sheet((u, v) => g(u, .94 + v * .06, .008), 220, 3, band), front = sheet((u, v) => { const p = g(.5 + (u - .5) * .03, .02 + v * .96, .009); return p; }, 2, 60, band);
  // Wide sleeves that hang open at the cuff.
  const arm = k => sleeve(k, [.4, 1.96], [.98, 1.2], v => .15 + .17 * v * v, .14);
  const sleeves = [-1, 1].map(k => sheet(arm(k), 40, 20, wool, { uvx: 3, uvy: 3 }));
  const cuffs = [-1, 1].map(k => sheet((u, v) => { const p = arm(k)(u, .9 + v * .1), q = arm(k)(u + .5, .9 + v * .1); return p.add(p.clone().sub(q).multiplyScalar(.015)); }, 40, 2, band));
  const inside = [-1, 1].map(k => ball(.3, dark(), { s: [.2, 1, .85], p: [k * .97, 1.2, 0], r: [0, 0, -k * .65] }));
  const hood = blob([.3, .22, .26], v => 1 + (n(v.x * 2 + 4, v.y * 2 + v.z) - .5) * .5 + .2 * Math.max(0, -v.y), wool, { p: [0, 2.0, -.2] }), collar = torus(.19, .06, wool, { r: [H2, 0, 0], s: [1.1, 1, .8], p: [0, 2.08, 0] });
  // A sash knotted at the hip, its ends hanging.
  const wy = o.y(.34), ww = o.w(.34), belt = sheet((u, v) => { const a = u * TAU; return V(Math.sin(a) * (ww[0] + .02), wy + .07 - v * .14 + .01 * Math.sin(a * 9), Math.cos(a) * (ww[1] + .02)); }, 60, 4, sash);
  const knot = ball(.075, sash, { p: [.22, wy, ww[1] * .86], s: [1, .9, .8] });
  const ends = [0, 1].map(i => sheet((u, v) => V(.2 + i * .07 + (u - .5) * (.1 + .04 * v) + .03 * Math.sin(v * 4 + i), wy - v * (.7 - i * .14), ww[1] * .86 + .05 + .16 * sstep(0, 1, v) + .02 * Math.sin(v * 6 + i * 2)), 4, 16, sash));
  const fringe = strands(26, (rr, i) => V(.15 + (i % 13) / 13 * .12 + (i > 12 ? .07 : 0), wy - (i > 12 ? .56 : .7), ww[1] * .86 + .21), (rr, i, a) => a.clone().add(V((rr() - .5) * .03, -.1 - rr() * .04, .01)), .005, sash, 9);
  return item('Robe', [part('robe', 'body', [0, 0, 0], [body, hood, collar].concat(sleeves, inside)), part('borders', 'trim', [0, -.4, .4], [hem, front].concat(cuffs)), part('sash', 'detail', [.3, 0, .6], [belt, knot].concat(ends, fringe))], { stand: true, yaw: .3, el: 10, fill: .86 });
}

export function belt(c) {
  const hide = leather(c.body, { double: true, bump: 1.2 }), br = metal(c.trim, { kind: 'cast', rough: .26 }), st = gem(c.detail, { round: true, edges: 0, inner: .5 });
  // Buckled into a loop, as worn, with the tongue hanging past the keeper.
  const A = .78, B = .56, H = .2, at = (a, y, out) => V(Math.sin(a) * (A + (out || 0)), y, Math.cos(a) * (B + (out || 0)));
  const strap = sheet((u, v) => at(.12 + u * (TAU - .08), .1 + v * H, 0), 150, 4, hide, { uvx: 8, uvy: .5 });
  const inner = sheet((u, v) => at(.12 + u * (TAU - .08), .1 + v * H, -.012), 150, 2, hide);
  // The tongue runs on past the buckle, flat against the strap, through a keeper.
  const tongue = sheet((u, v) => { const tp = sstep(.82, 1, u); return at(-u * .95, .1 + H * (.5 + (v - .5) * (1 - .85 * tp)), .024); }, 30, 4, hide);
  const frame = slab(hole(shape([[-.2, -.17], [.2, -.17], [.2, .17], [-.2, .17]]), [[-.13, -.1], [.13, -.1], [.13, .1], [-.13, .1]]), .04, .016, br, { p: [0, .2, B + .03] });
  const prong = rod([[.17, .2, B + .05], [0, .2, B + .075], [-.21, .2, B + .05]], .014, br, { n: 12, sides: 8 });
  const kp = at(-.5, .2, .03), keeper = rbox(.05, H + .06, .07, .015, br, { r: [0, -.5, 0], p: [kp.x, kp.y, kp.z] });
  // Mounts along the strap, each set with a small stone.
  const as = Array.from({ length: 9 }, (_, i) => .75 + i * .6);
  const mounts = as.map(a => { const p = at(a, .1 + H / 2, .012); return turned([[0, .02], [.04, .02], [.052, .008], [.056, 0]], br, { sm: 12, seg: 16, r: [H2, a, 0], p: [p.x, p.y, p.z] }); });
  const stones = as.map(a => { const p = at(a, .1 + H / 2, .034); return ball(.028, st, { p: [p.x, p.y, p.z], seg: 14, seg2: 10 }); });
  const tp = at(-.93, .2, .03), tip = rbox(.07, .07, .03, .012, br, { r: [0, -.93, 0], p: [tp.x, tp.y, tp.z] });
  return item('Belt', [part('strap', 'body', [0, 0, 0], [strap, inner, tongue]), part('buckle and mounts', 'trim', [0, .2, .5], [frame, prong, keeper, tip].concat(mounts)), part('stones', 'detail', [0, .5, 0], stones)], { stand: true, yaw: .3, el: 26, fill: .9 });
}

export function bracers(c) {
  const hide = leather(c.body, { double: true, bump: 1.2 }), steel = metal(c.trim, { rough: .36 }), cord = leather(c.detail);
  // A pair: each a hide sleeve for the forearm, splinted with steel and laced up the inside.
  const one = (x, z, turn) => {
    const R = v => .2 + .1 * v + .02 * Math.sin(v * Math.PI), gp = .2, pos = (u, v, out) => { const a = gp + u * (TAU - 2 * gp) + turn; return V(x + Math.sin(a) * (R(v) + (out || 0)), .04 + v * 1.1, z + Math.cos(a) * (R(v) + (out || 0)) * .88); };
    const cuff = sheet((u, v) => pos(u, v, 0), 60, 20, hide, { uvx: 3, uvy: 2 });
    const rolls = [0, 1].map(v => rod(Array.from({ length: 41 }, (_, i) => pos(i / 40, v, .008)), .022, hide, { sides: 8 }));
    const splints = [.3, .42, .5, .58, .7].map(u => { const pts = Array.from({ length: 13 }, (_, i) => pos(u, .08 + i / 12 * .84, .018)); return rod(pts, .03, steel, { sides: 6, s: [1, 1, 1] }); });
    const rivets = many(new THREE.SphereGeometry(.018, 8, 6), steel, [.3, .42, .5, .58, .7].map(u => [.1, .9].map(v => { const p = pos(u, v, .045); return { p: [p.x, p.y, p.z] }; })).flat());
    const lp = []; for (let i = 0; i < 8; i++) { const v = .08 + i / 7 * .84, k = i % 2; lp.push(pos(k ? .008 : .992, v, .012)); }
    const lace = rod(lp, .01, cord, { sides: 5 }), bow = [rod([lp[7], lp[7].clone().add(V(.1, .1, .04)), lp[7].clone().add(V(.16, -.06, .06))], .009, cord, { n: 10, sides: 5 }), rod([lp[7], lp[7].clone().add(V(-.1, .08, .04)), lp[7].clone().add(V(-.15, -.08, .06))], .009, cord, { n: 10, sides: 5 })];
    return { hide: [cuff].concat(rolls), steel: splints.concat([rivets]), cord: [lace].concat(bow) };
  };
  const a = one(-.34, 0, Math.PI + .5), b = one(.38, -.16, .9);
  return item('Bracers', [part('cuffs', 'body', [0, 0, 0], a.hide.concat(b.hide)), part('splints', 'trim', [0, .2, .5], a.steel.concat(b.steel)), part('laces', 'detail', [0, .5, .3], a.cord.concat(b.cord))], { stand: true, yaw: .35, el: 16, fill: .88 });
}

export function backpack(c) {
  const canvas = fine(cloth(c.body, { bump: 1.1, sheen: .2 }), 4), hide = leather(c.trim, { double: true }), wool = cloth(c.detail, { bump: .9, sheen: .5 }), br = metal('#b08a3a', { kind: 'cast' }), n = makeNoise(27, 16);
  const bag = blob([.62, .72, .4], v => { const k = 1 / Math.pow(Math.pow(Math.abs(v.x), 4) + Math.pow(Math.abs(v.y), 4.5) + Math.pow(Math.abs(v.z), 3), 1 / 3.8); return Math.min(1.4, k) * (1 + (n(v.x * 3 + 2, v.y * 3 + v.z * 2) - .5) * .08) * (1 + .12 * Math.max(0, -v.y)); }, canvas, { p: [0, .78, 0], seg: 90, seg2: 60 });
  const fz = (x, y) => .44 * Math.pow(Math.max(0, 1 - Math.pow(Math.abs(x) / .74, 3.4)), .5);
  // The flap comes over the top and half way down the front.
  const flap = sheet((u, v) => { const x = (u - .5) * 1.22, t = v, top = 1.52, drop = .66 * (1 - .22 * Math.pow(Math.abs(x) / .61, 2.4)); return t < .35 ? V(x, top + .04 * Math.sin(t / .35 * Math.PI), -.3 + (t / .35) * .3 + fz(x) * Math.sin(t / .35 * H2)) : V(x, top - (t - .35) / .65 * drop, fz(x) + .02 + .02 * Math.sin((t - .35) / .65 * Math.PI)); }, 40, 30, hide, { uvx: 3, uvy: 2 });
  const straps = [-1, 1].map(k => sheet((u, v) => V(k * .3 + (u - .5) * .1, 1.5 - v * .86, fz(k * .3) + .035 + .015 * Math.sin(v * Math.PI)), 3, 16, hide));
  const buckles = [-1, 1].map(k => [rbox(.15, .11, .03, .012, br, { p: [k * .3, .7, fz(k * .3) + .05] }), rbox(.09, .05, .04, .01, dark(), { p: [k * .3, .7, fz(k * .3) + .052] }), rod([[k * .3, .75, fz(.3) + .07], [k * .3, .66, fz(.3) + .07]], .008, br, { sides: 6 })]).flat();
  const pocket = blob([.24, .2, .14], v => Math.min(1.3, 1 / Math.pow(Math.pow(Math.abs(v.x), 4) + Math.pow(Math.abs(v.y), 4) + Math.pow(Math.abs(v.z), 2.4), 1 / 3.4)), canvas, { p: [.7, .5, 0], seg: 40, seg2: 28 }), pflap = sheet((u, v) => V(.7 + .13 + .08 * Math.sin(v * H2), .7 - v * .18, (u - .5) * .4), 6, 6, hide);
  // A blanket rolled and strapped on top.
  const roll = turned([[0, .62], [.14, .62], [.17, .6], [.17, -.6], [.14, -.62], [0, -.62]], wool, { sm: 0, seg: 40, r: [0, 0, H2], p: [0, 1.7, -.06] });
  const swirl = [-1, 1].map(k => rod(Array.from({ length: 40 }, (_, i) => { const a = i / 39 * TAU * 2.4, rr = .02 + i / 39 * .14; return V(k * .625, 1.7 + Math.sin(a) * rr, -.06 + Math.cos(a) * rr); }), .008, wool, { sides: 5 }));
  const ties = [-.34, .34].map(x => torus(.178, .016, hide, { r: [0, H2, 0], p: [x, 1.7, -.06] }));
  const shoulder = [-1, 1].map(k => rod([[k * .3, 1.48, -.36], [k * .38, 1.1, -.5], [k * .4, .6, -.46], [k * .34, .2, -.34]], .03, hide, { n: 20, sides: 8, s: [1, 1, .6] }));
  return item('Backpack', [part('pack', 'body', [0, 0, 0], [bag, pocket]), part('flap and straps', 'trim', [0, .2, .5], [flap, pflap].concat(straps, buckles, ties, shoulder)), part('blanket', 'detail', [0, .7, 0], [roll].concat(swirl))], { stand: true, yaw: .45, el: 14, fill: .88 });
}

export function brooch(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .22 }), sil = metal(c.trim, { kind: 'cast', rough: .26 }), st = gem(c.detail, { inner: .5, edges: .2 });
  // A ring brooch: a broad hoop worked like twisted rope, a stone at the head, and a long pin across it.
  const hoop = torus(.56, .075, gold, { s: [1, 1, .7] });
  const twist = (R, tw, ph) => rod(Array.from({ length: 241 }, (_, i) => { const a = i / 240 * TAU, b = a * tw + ph; return V(Math.cos(a) * (R + Math.cos(b) * .022), Math.sin(a) * (R + Math.cos(b) * .022), .05 + Math.sin(b) * .02); }), .02, gold, { sides: 6 });
  const ropes = [twist(.56, 30, 0), twist(.56, 30, Math.PI), twist(.45, 24, 0), twist(.67, 34, 0)];
  const grains = many(new THREE.SphereGeometry(.03, 10, 8), sil, Array.from({ length: 16 }, (_, i) => { const a = (i + .5) / 16 * TAU; return { p: [Math.cos(a) * .56, Math.sin(a) * .56, .09] }; }));
  const bezel = turned([[0, .05], [.15, .05], [.19, .03], [.2, 0]], gold, { sm: 16, seg: 32, r: [H2, 0, 0], p: [0, .56, .06] }), claws = many(new THREE.SphereGeometry(.026, 8, 6), sil, Array.from({ length: 6 }, (_, i) => { const a = i / 6 * TAU; return { p: [Math.cos(a) * .15, .56 + Math.sin(a) * .15, .12] }; }));
  const stone = M(cutStone(.14, .06, .04), st, { p: [0, .56, .11] });
  const small = [-1, 1].map(k => M(cutStone(.06, .03, .02), st, { p: [k * .5, -.26, .1] })), sb = [-1, 1].map(k => torus(.066, .016, gold, { p: [k * .5, -.26, .09] }));
  const pin = rod([[-.78, -.36, .1], [-.3, -.12, .14], [.3, .16, .14], [.92, .46, .08]], s => .03 * (1 - s * .8) + .004, sil, { n: 30, sides: 10 }), head = torus(.06, .022, sil, { p: [-.82, -.38, .1], r: [0, .4, 0] });
  return item('Brooch', [part('hoop', 'body', [0, 0, 0], [hoop, bezel].concat(ropes, sb)), part('pin and grains', 'trim', [0, -.2, .5], [pin, head, grains, claws]), part('stones', 'detail', [0, .4, .6], [stone].concat(small))], { yaw: .35, fill: .86 });
}

export function bracelet(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .2 }), sil = metal(c.trim, { kind: 'cast', rough: .24 }), st = gem(c.detail, { round: true, edges: 0, inner: .6 });
  // An open arm ring of twisted wires, ending in two knobs that nearly meet.
  const R = .6, open = .42, arc = a0 => Array.from({ length: 161 }, (_, i) => { const a = open + i / 160 * (TAU - 2 * open), b = a * 11 + a0; return V(Math.sin(a) * (R + Math.cos(b) * .045), .12 + Math.sin(b) * .045, Math.cos(a) * (R + Math.cos(b) * .045)); });
  const wires = [0, TAU / 3, TAU * 2 / 3].map(a0 => rod(arc(a0), .042, gold, { sides: 8 }));
  const ends = [-1, 1].map(k => { const a = k > 0 ? open : TAU - open, p = [Math.sin(a) * R, .12, Math.cos(a) * R]; return [turned([[0, .16], [.05, .15], [.085, .1], [.09, .04], [.07, 0], [.075, -.05], [.06, -.08]], sil, { sm: 30, seg: 24, r: [H2, a - k * H2, 0], p: p }), band(.07, 0, .05, .014, sil, { r: [H2, a - k * H2, 0], p: p })]; }).flat();
  const stones = [-1, 1].map(k => { const a = k > 0 ? open : TAU - open; return ball(.055, st, { p: [Math.sin(a) * R - k * Math.cos(a) * .18, .12, Math.cos(a) * R + k * Math.sin(a) * .18], seg: 16, seg2: 12 }); });
  const collars = [.9, 1.5, Math.PI, TAU - 1.5, TAU - .9].map(a => torus(.1, .02, sil, { r: [0, a + H2, 0], p: [Math.sin(a) * R, .12, Math.cos(a) * R] }));
  return item('Bracelet', [part('wires', 'body', [0, 0, 0], wires), part('knobs and collars', 'trim', [0, .3, .4], ends.concat(collars)), part('stones', 'detail', [0, .5, .6], stones)], { stand: true, yaw: .25, el: 30, fill: .9 });
}

export function spectacles(c) {
  const wire = metal(c.body, { kind: 'cast', rough: .24 }), lens = glass(c.trim, { tint: .3 }), cord = cloth(c.detail, { sheen: .8, bump: .4 });
  // Round lenses in wire rims, a sprung bridge, and arms folded half open.
  const rims = [-1, 1].map(k => torus(.3, .022, wire, { p: [k * .38, 0, 0] }));
  const panes = [-1, 1].map(k => turned([[0, .02], [.2, .016], [.288, 0], [.2, -.016], [0, -.02]], lens, { sm: 24, seg: 40, r: [H2, 0, 0], p: [k * .38, 0, 0] }));
  const bridge = rod([[-.09, .1, 0], [-.05, .2, .03], [.05, .2, .03], [.09, .1, 0]], .02, wire, { n: 16, sides: 8 });
  const pads = [-1, 1].map(k => ball(.03, wire, { p: [k * .1, -.05, -.06], s: [.6, 1.3, 1] }));
  const hinges = [-1, 1].map(k => rbox(.06, .07, .05, .012, wire, { p: [k * .7, .04, -.01] }));
  const arms = [-1, 1].map(k => rod([[k * .71, .04, -.02], [k * .66, .0, -.5], [k * .58, -.14, -.9], [k * .54, -.3, -1.06]], .016, wire, { n: 24, sides: 8 }));
  // A cord to hang them round the neck.
  const cordR = rod([[-.54, -.3, -1.06], [-.7, -.31, -1.3], [-.3, -.31, -1.5], [.1, -.31, -1.3], [.5, -.31, -1.5], [.74, -.31, -1.25], [.54, -.3, -1.06]], .012, cord, { n: 40, sides: 6 });
  const beads = [-1, 1].map(k => ball(.03, cord, { p: [k * .54, -.3, -1.07] }));
  return item('Spectacles', [part('frames', 'body', [0, 0, 0], rims.concat([bridge], pads, hinges, arms)), part('lenses', 'trim', [0, 0, .5], panes), part('cord', 'detail', [0, -.4, -.3], [cordR].concat(beads))], { stand: true, yaw: .5, el: 18, fill: .9 });
}

export function hood(c) {
  const wool = cloth(c.body, { bump: .9, sheen: .5 }), edgeM = cloth(c.trim, { sheen: 1 }), br = metal(c.detail, { kind: 'cast' }), n = makeNoise(53, 32);
  // The head of the hood: a deep cowl, open at the face, peaked at the back.
  const th0 = .95, shell = (th, ph, k) => { const peak = Math.pow(Math.max(0, -Math.cos(th)), 2) * Math.max(0, Math.sin(ph)) * .3, w = 1 + (n(Math.cos(ph) * 2 + 4, th * 2) - .5) * .1; return V(Math.sin(th) * Math.cos(ph) * .42 * w * k, 1.0 + Math.sin(th) * Math.sin(ph) * .5 * w * k + peak * .4, Math.cos(th) * .5 * k - peak * .5 + .06); };
  const cowl = sheet((u, v) => shell(th0 + v * (Math.PI - th0), u * TAU - H2, 1), 90, 40, wool, { uvx: 8, uvy: 4 });
  const shade = sheet((u, v) => shell(th0 + .04 + v * (Math.PI - th0 - .04), u * TAU - H2, .93), 60, 24, dark());
  const brim = rod(Array.from({ length: 61 }, (_, i) => shell(th0, i / 60 * TAU - H2, 1.01)), .04, edgeM, { sides: 8 });
  // The mantle: a short cape over the shoulders, cut in points along its edge.
  const cape = sheet((u, v) => { const a = Math.PI + u * TAU, r = .34 + .56 * Math.pow(v, .8) + Math.sin(a * 9 + n(u * 5, 2) * 3) * .05 * v, dag = .1 * Math.abs(Math.sin(a * 6)) * sstep(.75, 1, v); return V(Math.sin(a) * r * 1.1, .56 - .5 * Math.pow(v, 1.5) - dag + .03 * Math.sin(a * 9) * v, Math.cos(a) * r * .86); }, 180, 24, wool, { uvx: 8, uvy: 2 });
  const hemE = rod(Array.from({ length: 241 }, (_, i) => { const u = i / 240, a = Math.PI + u * TAU, r = .34 + .56 + Math.sin(a * 9 + n(u * 5, 2) * 3) * .05 + .008; return V(Math.sin(a) * r * 1.1, .06 - .1 * Math.abs(Math.sin(a * 6)) + .03 * Math.sin(a * 9), Math.cos(a) * r * .86); }), .016, edgeM, { sides: 6 });
  // The tail of the hood, hanging down the back.
  const tail = rod([[0, 1.42, -.4], [0, 1.3, -.72], [.06, .8, -.86], [.1, .2, -.9], [.08, -.2, -.86]], s => .1 * (1 - s * .8) + .012, wool, { n: 30, sides: 10 });
  const clasp = turned([[0, .04], [.06, .036], [.085, .015], [.09, 0]], br, { sm: 16, seg: 24, r: [H2, 0, 0], p: [0, .5, .36] }), pin = rod([[-.12, .46, .4], [.12, .54, .4]], .009, br, { sides: 6 });
  return item('Hood', [part('hood', 'body', [0, 0, 0], [cowl, cape, tail, shade]), part('edging', 'trim', [0, 0, .4], [brim, hemE]), part('clasp', 'detail', [0, -.2, .8], [clasp, pin])], { stand: true, yaw: .45, el: 12, fill: .88 });
}
