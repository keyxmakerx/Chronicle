// More weapons: a greatsword, a rapier, a halberd, a trident, a scythe, a sickle, a whip, a sling, a club, throwing knives and a double axe.
import { THREE, V, M, X, Y, Z, part, item, turned, rod, ball, torus, cyl, rbox, slab, blob, sheet, many, aim, blade, band, wrap, bar, strands, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, matte, rock } from '../mats.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
const sstep = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
// A blade's point: full width until `from`, then drawn in to nothing.
const point = (s, from) => Math.sqrt(Math.max(0, 1 - Math.pow(Math.max(0, (s - from) / (1 - from)), 3)));

export function greatsword(c) {
  const steel = metal(c.body, { rough: .28 }), fit = metal(c.trim, { kind: 'cast' }), hide = leather(c.detail), L = 3.9;
  // A narrow, unsharpened length above the guard, then the blade proper.
  const bl = M(blade(L, s => (.07 + .036 * sstep(.11, .16, s) - .03 * s) * point(s, .88), s => (.021 - .011 * s) * (1 - Math.max(0, (s - .9) / .1) * .7), { fuller: s => .001 + .03 * sstep(.15, .2, s) * (1 - sstep(.55, .66, s)), n: 72, uvy: 6 }), steel);
  const lugs = [-1, 1].map(k => slab([[.055 * k, .4], [.21 * k, .5], [.055 * k, .62]], .028, .01, steel, { bseg: 2 }));
  const guard = turned([[0, -.66], [.03, -.655], [.046, -.62], [.034, -.56], [.026, -.3], [.03, -.1], [.05, -.06], [.056, 0], [.05, .06], [.03, .1], [.026, .3], [.034, .56], [.046, .62], [.03, .655], [0, .66]], fit, { sm: 96, seg: 32, r: [0, 0, H2], s: [1, 1, .85] });
  const gr = y => .04 + .009 * Math.sin(Math.min(1, (-.06 - y) / .52) * Math.PI) + .006 * sstep(-.6, -1.1, y);
  const grip = wrap(-.06, -1.12, gr, hide, { turns: 64, sz: .85, cord: .008 });
  const risers = [-.07, -.58, -1.11].map(y => torus(gr(y) + .008, .011, fit, { r: [H2, 0, 0], s: [1, 1, .85], p: [0, y, 0] }));
  const pom = turned([[.04, -1.12], [.05, -1.16], [.085, -1.24], [.09, -1.3], [.07, -1.37], [.03, -1.41], [0, -1.42]], fit, { sm: 40, seg: 28 });
  return item('Greatsword', [part('blade', 'body', [0, .4, 0], [bl].concat(lugs)), part('guard and pommel', 'trim', [0, -.1, .3], [guard, pom].concat(risers)), part('grip', 'detail', [0, -.5, 0], grip)], { yaw: -.8, fill: .98 });
}

export function rapier(c) {
  const steel = metal(c.body, { rough: .24 }), fit = metal(c.trim, { kind: 'cast', rough: .26 }), wire = metal(c.detail, { kind: 'cast', rough: .36 }), L = 3.3;
  const bl = M(blade(L, s => .036 * (1 - .55 * s) * point(s, .93), s => .015 * (1 - .5 * s), { fuller: s => s < .3 ? .01 : 0, n: 64, uvy: 6 }), steel, { p: [0, -.1, 0] });
  // The cup: a thin bowl with its hollow toward the hand.
  const cupP = []; for (let i = 0; i <= 18; i++) { const a = i / 18 * H2; cupP.push([.024 + .216 * Math.sin(a), .02 - .22 * (1 - Math.cos(a))]); } for (let i = 18; i >= 0; i--) { const a = i / 18 * H2; cupP.push([.024 + .204 * Math.sin(a), .006 - .21 * (1 - Math.cos(a))]); }
  const cup = turned(cupP, fit, { sm: 0, seg: 56 }), lip = torus(.238, .011, fit, { r: [H2, 0, 0], p: [0, -.2, 0] });
  const quill = rod([[-.46, -.2, 0], [-.2, -.215, 0], [0, -.22, 0], [.2, -.215, 0], [.46, -.2, 0]], s => .014 + .01 * Math.abs(s - .5), fit, { n: 24, sides: 10 });
  const knobs = [-1, 1].map(k => ball(.03, fit, { p: [.47 * k, -.2, 0] }));
  const bow = rod([[-.3, -.21, 0], [-.37, -.3, 0], [-.34, -.56, 0], [-.2, -.8, 0], [-.05, -.87, 0]], s => .013 + .006 * Math.sin(s * Math.PI), fit, { n: 30, sides: 10 });
  const gr = y => .031 + .009 * Math.sin((-.22 - y) / .6 * Math.PI);
  const grip = wrap(-.22, -.82, gr, wire, { turns: 46, cord: .006 });
  const pom = turned([[.03, -.82], [.05, -.86], [.062, -.92], [.05, -.98], [.02, -1.01], [0, -1.015]], fit, { sm: 36, seg: 24 });
  return item('Rapier', [part('blade', 'body', [0, .4, 0], [bl]), part('cup and bow', 'trim', [-.3, -.1, .3], [cup, lip, quill, bow, pom].concat(knobs)), part('grip', 'detail', [.3, -.4, .2], grip)], { yaw: -.7, fill: .98 });
}

export function halberd(c) {
  const steel = metal(c.body, { rough: .34 }), wd = wood(c.trim), silk = cloth(c.detail, { sheen: 1 }), r = rng(11);
  const shaft = turned([[.036, -1.5], [.04, -.6], [.04, 1.7], [.036, 2.2]], wd, { sm: 0, seg: 24 });
  // The axe blade: a crescent with its horns swept back toward the shaft.
  const axeB = slab([[.04, 1.72], [.2, 1.66], [.42, 1.42], [.6, 1.62], [.68, 1.95], [.6, 2.3], [.42, 2.5], [.2, 2.26], [.04, 2.2]], .04, .019, steel, { curved: false, bseg: 3 });
  const fluke = slab([[-.04, 1.84], [-.3, 1.82], [-.52, 1.62], [-.4, 1.96], [-.04, 2.1]], .036, .016, steel, { bseg: 3 });
  const spike = M(blade(1.0, s => .016 + .07 * Math.pow(Math.sin(Math.min(1, s * 1.3 + .1) * H2), .8) * Math.pow(1 - s, .8), s => .026 * (1 - s * .8), { n: 40 }), steel, { p: [0, 2.26, 0] });
  const socket = turned([[.044, 1.6], [.052, 1.66], [.046, 1.72], [.044, 2.2], [.03, 2.3]], steel, { sm: 30, seg: 24 });
  const langets = [-1, 1].map(k => rbox(.02, .7, .05, .008, steel, { p: [0, 1.26, k * .036] }));
  const tass = []; for (let i = 0; i < 40; i++) { const a = r() * TAU, l = .3 + r() * .22, sp = .05 + r() * .08; tass.push(rod([[Math.cos(a) * .044, 1.58, Math.sin(a) * .044], [Math.cos(a) * (.06 + sp), 1.5, Math.sin(a) * (.06 + sp)], [Math.cos(a) * (.06 + sp * 1.3), 1.58 - l, Math.sin(a) * (.06 + sp * 1.3)]], .008, silk, { n: 8, sides: 5 })); }
  const tie = torus(.05, .013, silk, { r: [H2, 0, 0], p: [0, 1.58, 0] });
  const butt = turned([[.04, -1.4], [.047, -1.46], [.04, -1.56], [.014, -1.74], [0, -1.77]], steel, { sm: 24 });
  return item('Halberd', [part('shaft', 'trim', [0, -.2, 0], [shaft]), part('head', 'body', [.2, .4, 0], [axeB, fluke, spike, socket, butt].concat(langets)), part('tassel', 'detail', [-.3, .1, .3], tass.concat([tie]))], { yaw: .3, fill: 1.0 });
}

export function trident(c) {
  const steel = metal(c.body, { kind: 'forged', rough: .4 }), wd = wood(c.trim), cord = cloth(c.detail, { sheen: .3, bump: .9 });
  const shaft = turned([[.036, -1.3], [.042, -.4], [.042, 1.6], [.038, 1.95]], wd, { sm: 0, seg: 24 });
  const tineR = s => .05 * (1 - s * .72) + .005;
  const mid = rod([[0, 1.9, 0], [0, 2.4, 0], [0, 3.2, 0]], tineR, steel, { n: 30, sides: 12 });
  const sides = [-1, 1].map(k => rod([[0, 2.02, 0], [.28 * k, 2.03, 0], [.46 * k, 2.18, 0], [.5 * k, 2.46, 0], [.48 * k, 3.0, 0]], s => .048 * (1 - s * .7) + .005, steel, { n: 36, sides: 12 }));
  // Barbs: a small hook behind each point, so what it sticks stays stuck.
  const barb = (x, y, k) => slab([[0, 0], [.13 * k, -.24], [.03 * k, -.17], [0, -.32]], .026, .009, steel, { p: [x, y, 0], bseg: 2 });
  const barbs = [barb(.012, 3.06, 1), barb(-.012, 3.06, -1), barb(.49, 2.88, 1), barb(-.49, 2.88, -1)];
  const socket = turned([[.044, 1.62], [.054, 1.68], [.048, 1.76], [.05, 1.98], [.062, 2.03], [.04, 2.08]], steel, { sm: 30, seg: 24 });
  const bind = wrap(.3, -.5, () => .043, cord, { turns: 34, cord: .009 });
  const lash = rod([[.04, -.5, 0], [.1, -.62, .03], [.08, -.8, .05], [.13, -.96, .04]], .011, cord, { n: 14, sides: 6 }), bead = ball(.03, cord, { p: [.13, -.98, .04] });
  const butt = turned([[.038, -1.2], [.048, -1.26], [.042, -1.36], [.012, -1.54], [0, -1.57]], steel, { sm: 24 });
  return item('Trident', [part('shaft', 'trim', [0, -.2, 0], [shaft]), part('tines', 'body', [0, .4, 0], [mid, socket, butt].concat(sides, barbs)), part('binding', 'detail', [.3, -.3, .2], bind.concat([lash, bead]))], { yaw: -.3, fill: 1.0 });
}

export function scythe(c) {
  const steel = metal(c.body, { rough: .32 }), wd = wood(c.trim, { bump: .9 }), hide = leather(c.detail);
  // The snath: a long shaft with a slow double bend, so the blade lies flat to the ground when swung.
  const sp = path([[.16, -2.5, 0], [.06, -1.4, 0], [-.06, -.2, 0], [0, 1.0, 0], [.14, 1.9, 0], [.2, 2.3, 0]], 60);
  const snath = M(tube(sp, s => .044 - .008 * s, 12), wd);
  const grips = [[.045, -1.0], [-.04, .35]].map(([x, y]) => [cyl(.03, .034, .34, wd, { r: [0, 0, H2], p: [x - .2, y, 0] }), ball(.042, wd, { p: [x - .38, y, 0], s: [.8, 1, 1] })]).flat();
  const bindings = [[.045, -1.0], [-.04, .35]].map(([x, y]) => wrap(y - .09, y + .09, () => .046, hide, { turns: 9, cord: .011 }).map(m => { m.geometry.translate(x, 0, 0); return m; })).flat();
  // The blade: long, thin, curved like a beak, sharp on the inside.
  const L = 2.5, g = blade(L, s => (.03 + .15 * Math.pow(1 - s, .7)) * point(s, .97), s => .014 * (1 - s * .6), { n: 60 });
  { const p = g.attributes.position; for (let i = 0; i < p.count; i++) { const y = p.getY(i), x = p.getX(i), s = y / L; p.setX(i, (x > 0 ? x * .3 : x * 1.7) - .5 * Math.pow(s, 1.8) * L * .36); } g.computeVertexNormals(); }
  const bl = M(g, steel, { r: [0, 0, H2 + .1], p: [.16, 2.28, 0] });
  const tang = rbox(.12, .2, .06, .02, steel, { p: [.2, 2.26, 0] }), ringB = torus(.06, .014, steel, { r: [H2, 0, 0], p: [.17, 2.08, 0] });
  return item('Scythe', [part('snath', 'trim', [.2, -.3, 0], [snath].concat(grips)), part('blade', 'body', [-.3, .4, 0], [bl, tang, ringB]), part('bindings', 'detail', [.4, 0, .3], bindings)], { yaw: .35, fill: .98 });
}

export function sickle(c) {
  const steel = metal(c.body, { rough: .3 }), wd = wood(c.trim, { polish: .3 }), cord = leather(c.detail);
  const handle = turned([[.05, -.95], [.062, -.9], [.056, -.6], [.046, -.3], [.05, -.1], [.04, 0]], wd, { sm: 40, seg: 28 });
  // The blade leaves the handle straight, then sweeps round in three quarters of a circle.
  const arc = [V(0, -.02, 0), V(0, .14, 0)]; for (let i = 0; i <= 44; i++) { const a = -.5 + i / 44 * 4.1; arc.push(V(-.62 + Math.cos(a) * .62 + .08, .44 + Math.sin(a) * .6, 0)); }
  const bl = bar(arc, s => .05 + .13 * Math.sin(Math.min(1, s * 1.5) * H2) * Math.pow(1 - s, .55), s => .03 * (1 - s * .7) + .003, steel);
  const ferr = band(.043, -.04, .12, .012, metal(c.body, { kind: 'cast' }));
  const bind = wrap(-.84, -.5, y => .06 - (y + .84) * .012, cord, { turns: 14, cord: .011 });
  const loop = rod([[0, -.95, 0], [.1, -1.1, .02], [0, -1.26, .03], [-.1, -1.1, .02], [0, -.95, 0]], .012, cord, { n: 24, sides: 6 });
  return item('Sickle', [part('handle', 'trim', [.2, -.3, 0], [handle]), part('blade', 'body', [-.2, .3, 0], bl.concat([ferr])), part('binding', 'detail', [.3, -.4, .3], bind.concat([loop]))], { yaw: -.35, fill: .92 });
}

export function whip(c) {
  const hide = leather(c.body, { bump: 1.3 }), fit = metal(c.trim, { kind: 'cast' }), silk = cloth(c.detail, { sheen: .6 }), r = rng(9);
  const handle = wrap(-1.0, -.3, y => .05 - (y + 1.0) * .014, hide, { turns: 26, cord: .012 });
  const knob = turned([[0, -1.1], [.04, -1.09], [.066, -1.05], [.06, -1.0], [.05, -.98]], fit, { sm: 24, seg: 24 }), collar = band(.042, -.3, .07, .012, fit);
  // The thong leaves the handle and is coiled three times, thinning all the way to the fall.
  const pts = [V(0, -.3, 0), V(.02, -.05, 0), V(.12, .22, .01)];
  for (let i = 0; i <= 150; i++) { const s = i / 150, a = -.5 + s * TAU * 3.05, R = .68 - .12 * s; pts.push(V(-.46 + Math.cos(a) * R * 1.02 + .02 * Math.sin(a * 3), .52 + Math.sin(a) * R * .92, .03 + s * .16 + .02 * Math.sin(a * 2))); }
  const end = pts[pts.length - 1].clone(); pts.push(end.clone().add(V(.12, -.2, .02)), end.clone().add(V(.2, -.52, .02)), end.clone().add(V(.16, -.86, .01)));
  const thong = M(tube(path(pts, 420), s => .04 - .03 * Math.pow(s, .8), 9), hide);
  const tip = end.clone().add(V(.16, -.86, .01));
  const cracker = strands(14, () => tip.clone(), (rr) => tip.clone().add(V((rr() - .5) * .14, -.26 - rr() * .1, (rr() - .5) * .06)), .005, silk, 5);
  const knot = ball(.022, silk, { p: [tip.x, tip.y, tip.z] });
  const loop = rod([[0, -1.1, 0], [.12, -1.24, .02], [.02, -1.44, .03], [-.1, -1.26, .02], [0, -1.1, 0]], .011, silk, { n: 24, sides: 6 });
  return item('Whip', [part('thong and handle', 'body', [0, 0, 0], handle.concat([thong])), part('knob and collar', 'trim', [.3, -.4, .2], [knob, collar]), part('cracker and loop', 'detail', [.4, -.2, .3], cracker.concat([knot, loop]))], { yaw: -.25, fill: .94 });
}

export function sling(c) {
  const hide = leather(c.body, { double: true, bump: 1.2 }), stone = rock(c.trim, { rough: .7, bump: 1.2 }), yarn = cloth(c.detail, { sheen: .3, bump: 1 }), n = makeNoise(13, 16);
  // Held by its two cords, the cradle hangs low with a stone in it.
  const cordL = rod([[-.06, .34, 0], [-.1, -.1, .02], [-.2, -.5, 0], [-.34, -.86, 0]], .017, yarn, { n: 30, sides: 7 });
  const cordR = rod([[.07, .22, .02], [.1, -.1, -.02], [.2, -.5, 0], [.34, -.86, 0]], .017, yarn, { n: 30, sides: 7 });
  const cradle = sheet((u, v) => { const x = (u - .5) * .74, w = Math.pow(Math.sin(Math.PI * u), .6), z = (v - .5) * .5 * w; return V(x, -.86 - .2 * Math.sin(Math.PI * u) + .12 * Math.pow(Math.abs(v - .5) * 2, 2) * w, z); }, 30, 14, hide);
  const slits = [-1, 1].map(k => rbox(.06, .024, .1, .008, hide, { p: [.35 * k, -.865, 0] }));
  const shot = blob([.14, .12, .13], v => 1 + (n(v.x * 2 + 3, v.y * 2 + v.z) - .5) * .18, stone, { p: [0, -.93, 0], seg: 40, seg2: 28 });
  const spare = [[-.6, .14, .11], [.58, -.1, .1], [.76, .2, .08]].map(([x, z, s], i) => blob([s * 1.15, s, s], v => 1 + (n(v.x * 2 + i * 7, v.y * 2 + v.z) - .5) * .2, stone, { p: [x, -1.07 + s, z], seg: 32, seg2: 22 }));
  // One cord ends in a finger loop, the other in a knot to let go of.
  const loop = torus(.08, .017, yarn, { p: [-.06, .42, 0], s: [.8, 1, 1] }), knot = ball(.04, yarn, { p: [.07, .24, .02] });
  const whip = [[-.2, -.5], [.2, -.5]].map(([x, y]) => torus(.022, .011, yarn, { r: [H2, 0, 0], p: [x, y, 0] }));
  return item('Sling', [part('cradle', 'body', [0, -.2, .3], [cradle].concat(slits)), part('stones', 'trim', [.3, -.3, 0], [shot].concat(spare)), part('cords', 'detail', [0, .4, 0], [cordL, cordR, loop, knot].concat(whip))], { stand: true, yaw: .3, fill: .9 });
}

export function club(c) {
  const wd = wood(c.body, { bump: 1.1 }), iron = metal(c.trim, { kind: 'forged', rough: .5 }), hide = leather(c.detail), r = rng(23);
  const body = turned([[0, 1.22], [.1, 1.2], [.2, 1.08], [.235, .86], [.21, .5], [.15, .1], [.085, -.4], [.062, -.9], [.07, -1.22], [.055, -1.28], [0, -1.29]], wd, { sm: 80, seg: 40 });
  const rad = y => y > .86 ? .235 - (y - .86) * .1 : .235 - (.86 - y) * .11;
  const bands = [.34, .98].map(y => band(rad(y) - .012, y, .09, .018, iron));
  // Studs in rings round the head: square-footed, drawn to a blunt point.
  const sg = new THREE.ConeGeometry(.04, .11, 4, 1); sg.rotateX(H2); const at = [];
  [.5, .66, .82].forEach((y, j) => { for (let i = 0; i < 7; i++) { const a = (i + j * .5) / 7 * TAU + (r() - .5) * .1, R = rad(y) + .035; at.push({ p: [Math.cos(a) * R, y, Math.sin(a) * R], r: [0, H2 - a, 0] }); } });
  const studs = many(sg, iron, at);
  const grip = wrap(-1.18, -.52, y => .064 + (y + 1.18) * .028, hide, { turns: 24, cord: .012 });
  const thong = rod([[0, -1.28, 0], [.14, -1.42, .02], [.04, -1.68, .04], [-.12, -1.46, .02], [0, -1.28, 0]], .013, hide, { n: 24, sides: 6 });
  return item('Studded club', [part('club', 'body', [0, 0, 0], [body]), part('studs and bands', 'trim', [.3, .4, .3], [studs].concat(bands)), part('grip', 'detail', [.3, -.4, .2], grip.concat([thong]))], { yaw: .6, fill: .95 });
}

export function knives(c) {
  const steel = metal(c.body, { rough: .26 }), cord = cloth(c.trim, { sheen: .2, bump: 1 }), silk = cloth(c.detail, { sheen: 1, bump: .3 });
  // Each knife is one piece of steel: leaf blade, flat tang, a ring to hang it by.
  const one = (ang, z, k) => {
    const q = new THREE.Matrix4().makeTranslation(0, -1.4, z).multiply(new THREE.Matrix4().makeRotationZ(ang)).multiply(new THREE.Matrix4().makeTranslation(0, .5, 0)), put = m => { m.geometry.applyMatrix4(q); return m; };
    const hang = m => { m.geometry.translate(-Math.sin(ang) * .52, -1.4 + Math.cos(ang) * .52, z); return m; };
    const bl = M(blade(1.0, s => (.02 + .1 * Math.pow(Math.sin(Math.min(1, s * 1.25 + .12) * H2), 1.2) * Math.pow(1 - s, .62)), s => .012 * (1 - s * .6), { n: 40 }), steel, { p: [0, .62, 0] });
    const tang = rbox(.06, .56, .016, .006, steel, { p: [0, .36, 0] });
    const ringK = torus(.06, .016, steel, { p: [0, .02, 0] });
    const bind = wrap(.12, .58, () => .034, cord, { turns: 16, sz: .5, cord: .009 });
    const tail = sheet((u, v) => V((u - .5) * .05 + .06 * Math.sin(v * 3 + k) * v, -.04 - v * .5, .01 + .03 * Math.sin(v * 5 + k * 2) * v), 4, 16, silk);
    return { steel: [bl, tang, ringK].map(put), bind: bind.map(put), tail: hang(tail) };
  };
  const ks = [one(.5, -.03, 0), one(0, 0, 1.7), one(-.5, .03, 3.1)];
  return item('Throwing knives', [part('knives', 'body', [0, .3, 0], ks.map(k => k.steel).flat()), part('bindings', 'trim', [0, -.1, .4], ks.map(k => k.bind).flat()), part('ribbons', 'detail', [0, -.5, .2], ks.map(k => k.tail))], { yaw: .25, fill: .92 });
}

export function doubleaxe(c) {
  const steel = metal(c.body, { rough: .36 }), wd = wood(c.trim), hide = leather(c.detail);
  const haft = turned([[.052, -1.9], [.047, -.6], [.044, .7], [.05, 1.12]], wd, { sm: 30, seg: 28, s: [1, 1, .72] });
  // Two heads back to back, each thick at the eye and drawn out thin to a wide curved edge.
  const head = k => { const N = 30, rings = []; for (let i = 0; i <= N; i++) { const s = i / N, x = s * .82, hh = .14 + .56 * Math.pow(s, 1.4), t = .078 * Math.pow(1 - s, 1.6) + .002, bow = Math.pow(s, 6) * .14, pts = []; for (let j = 0; j < 16; j++) { const a = j / 16 * TAU * k, yy = Math.sin(a) * hh, zz = Math.cos(a) * t; pts.push(V(k * (x + (1 - Math.pow(Math.abs(Math.sin(a)), 2)) * bow), .8 + yy, zz)); } rings.push(pts); } return M(loft(rings), steel); };
  const eye = rbox(.15, .36, .17, .03, steel, { p: [0, .8, 0] });
  const spike = turned([[.05, 1.1], [.058, 1.14], [.04, 1.2], [.018, 1.42], [0, 1.46]], steel, { sm: 24, seg: 20 });
  const lang = [-1, 1].map(k => rbox(.02, .5, .05, .008, steel, { p: [0, .4, k * .038] }));
  const grip = wrap(-.9, -1.8, () => .05, hide, { turns: 34, sz: .72 }), cap = turned([[.052, -1.88], [.064, -1.92], [.058, -2.0], [0, -2.03]], steel, { sm: 24, s: [1, 1, .75] });
  return item('Battleaxe', [part('haft', 'trim', [0, -.1, 0], [haft]), part('heads', 'body', [0, .4, 0], [head(1), head(-1), eye, spike, cap].concat(lang)), part('grip', 'detail', [.3, -.3, .2], grip)], { yaw: .5, fill: .97 });
}
