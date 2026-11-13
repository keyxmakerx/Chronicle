// Weapons. One unit is about 30 cm; each is modelled standing in the XY plane, front toward +Z.
import { THREE, V, M, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, slab, shape, circle, blob, sheet, chain, many, aim, blade, wrap, band, tube, path, loft, ring, makeNoise, fbm, rng, X, Y, Z } from '../kit.js';
import { metal, wood, leather, cloth, gem, bone, matte, dark } from '../mats.js';

export function sword(c) {
  const steel = metal(c.body), fit = metal(c.trim, { kind: 'cast' }), hide = leather(c.detail);
  const bl = M(blade(3.0, s => { const tip = Math.max(0, (s - .9) / .1); return .084 * (1 - .3 * Math.min(s, .9)) * Math.sqrt(Math.max(0, 1 - tip * tip * tip)); }, s => (.0165 - .008 * s) * (1 - Math.max(0, (s - .9) / .1) * .75), { fuller: s => .001 + .028 * Math.min(1, Math.max(0, (.64 - s) / .08)), n: 64, uvy: 5 }), steel);
  const guard = turned([[0, -.39], [.022, -.385], [.034, -.36], [.03, -.33], [.021, -.24], [.024, -.12], [.03, -.07], [.05, -.055], [.052, 0], [.05, .055], [.03, .07], [.024, .12], [.021, .24], [.03, .33], [.034, .36], [.022, .385], [0, .39]], fit, { sm: 96, seg: 40, r: [0, 0, Math.PI / 2], s: [1, 1, .8] });
  const gr = y => { const k = (y + .058) / -.642; return .036 + .012 * Math.sin(Math.min(1, k * 1.25) * Math.PI) * (1 - k * .25); };
  const grip = wrap(-.058, -.70, gr, hide, { turns: 46, sz: .82, cord: .0078 });
  const risers = [-.066, -.692].map(y => torus(gr(y) + .006, .009, hide, { r: [Math.PI / 2, 0, 0], s: [1, 1, .82], p: [0, y, 0] }));
  const pom = turned([[0, .05], [.03, .05], [.036, .042], [.06, .04], [.082, .026], [.088, .012], [.088, -.012], [.082, -.026], [.06, -.04], [.036, -.042], [.03, -.05], [0, -.05]], fit, { sm: 60, r: [Math.PI / 2, 0, 0], p: [0, -.782, 0] });
  const peen = ball(.016, fit, { s: [1, .6, 1], p: [0, -.872, 0] });
  return item('Longsword', [part('blade', 'body', [0, .34, 0], [bl]), part('guard', 'trim', [0, 0, 0], [guard]), part('grip', 'detail', [0, -.3, 0], grip.concat(risers)), part('pommel', 'trim', [0, -.62, 0], [pom, peen])], { yaw: -.8, fill: .98 });
}

export function dagger(c) {
  const steel = metal(c.body), fit = metal(c.trim, { kind: 'cast' }), wd = wood('#4a2f1e', { polish: .3 }), st = gem(c.detail);
  const bl = M(blade(1.0, s => .1 * Math.pow(1 - Math.pow(s, 2.2), .8) + .001, s => .016 * (1 - s * .7), { n: 48 }), steel);
  const guard = M(new THREE.CapsuleGeometry(.03, .34, 8, 20), fit, { r: [0, 0, Math.PI / 2], s: [1, 1.15, .9], p: [0, -.03, 0] });
  const block = rbox(.13, .07, .075, .02, fit, { p: [0, -.03, 0] });
  const hilt = turned([[.034, -.06], [.04, -.12], [.047, -.24], [.044, -.34], [.036, -.44], [.04, -.47]], wd, { sm: 40, s: [1, 1, .85] });
  const rings = [-.16, -.26, -.36].map(y => torus(.047, .006, fit, { r: [Math.PI / 2, 0, 0], s: [1, 1, .85], p: [0, y, 0] }));
  const pom = turned([[.036, -.46], [.06, -.5], [.066, -.55], [.05, -.6], [0, -.62]], fit, { sm: 30 });
  const stone = M(new THREE.OctahedronGeometry(.036, 0), st, { s: [1, 1.2, .8], p: [0, -.545, .05] });
  return item('Dagger', [part('blade', 'body', [0, .28, 0], [bl]), part('guard', 'trim', [0, 0, 0], [guard, block]), part('hilt', 'trim', [0, -.22, 0], [hilt].concat(rings)), part('pommel', 'trim', [0, -.42, 0], [pom]), part('pommel stone', 'detail', [0, -.42, .3], [stone])], { yaw: .7, fill: .95 });
}

export function spear(c) {
  const steel = metal(c.body), wd = wood(c.trim), hair = cloth(c.detail, { sheen: 1 }), r = rng(3);
  const shaft = turned([[.03, -2.6], [.034, -1], [.034, 2.2], [.03, 2.6]], wd, { sm: 0, seg: 24 });
  const head = M(blade(.95, s => .012 + .13 * Math.pow(Math.sin(Math.min(1, s * 1.15 + .08) * Math.PI * .5), .8) * Math.pow(1 - s, .7), s => .03 * (1 - s * .85), { n: 48 }), steel, { p: [0, 2.9, 0] });
  const socket = turned([[.04, 2.5], [.046, 2.56], [.04, 2.62], [.036, 2.9], [.02, 2.96]], steel, { sm: 30 });
  const tass = []; for (let i = 0; i < 46; i++) { const a = r() * Math.PI * 2, l = .28 + r() * .2, sp = .05 + r() * .09; tass.push(rod([[Math.cos(a) * .04, 2.5, Math.sin(a) * .04], [Math.cos(a) * (.05 + sp), 2.42, Math.sin(a) * (.05 + sp)], [Math.cos(a) * (.05 + sp * 1.3), 2.5 - l, Math.sin(a) * (.05 + sp * 1.3) - .02]], .008, hair, { n: 8, sides: 5 })); }
  const tie = torus(.044, .012, hair, { r: [Math.PI / 2, 0, 0], p: [0, 2.5, 0] });
  const butt = turned([[.034, -2.5], [.04, -2.56], [.036, -2.66], [.012, -2.86], [0, -2.9]], steel, { sm: 24 });
  return item('Spear', [part('shaft', 'trim', [0, 0, 0], [shaft]), part('head', 'body', [0, .4, 0], [head]), part('socket', 'trim', [0, .2, 0], [socket]), part('tassel', 'detail', [.25, .1, 0], tass.concat([tie])), part('butt spike', 'body', [0, -.4, 0], [butt])], { yaw: -.82, fill: 1.0 });
}

export function mace(c) {
  const steel = metal(c.body, { kind: 'forged', rough: .42 }), wd = wood(c.trim), hide = leather(c.detail);
  const haft = turned([[.04, -1.5], [.045, -.4], [.04, .5]], wd, { sm: 0, seg: 28 });
  const core = turned([[.05, .42], [.075, .5], [.085, .8], [.07, 1.08], [.04, 1.14]], steel, { sm: 40 });
  const fl = []; for (let i = 0; i < 7; i++) fl.push(slab([[.06, .46], [.2, .6], [.27, .78], [.25, .9], [.14, 1.06], [.06, 1.1]], .03, .012, steel, { r: [0, i / 7 * Math.PI * 2, 0], bseg: 2 }));
  const fin = turned([[.04, 1.12], [.055, 1.16], [.03, 1.22], [.014, 1.34], [0, 1.37]], steel, { sm: 30 });
  const collar = band(.047, .42, .07, .014, steel);
  const grip = wrap(-.72, -1.42, () => .046, hide, { turns: 30 });
  const cap = turned([[.046, -1.42], [.062, -1.46], [.06, -1.54], [.03, -1.58], [0, -1.58]], steel, { sm: 30 });
  return item('Flanged mace', [part('haft', 'trim', [0, 0, 0], [haft]), part('flanges', 'body', [0, .35, 0], fl.concat([core])), part('finial and collar', 'trim', [0, .6, 0], [fin, collar]), part('grip', 'detail', [0, -.25, 0], grip), part('cap', 'trim', [0, -.5, 0], [cap])], { yaw: .75, fill: .98 });
}

export function axe(c) {
  const steel = metal(c.body, { rough: .38 }), wd = wood(c.trim), hide = leather(c.detail);
  const haft = turned([[.05, -1.6], [.046, -.6], [.043, .6], [.05, .98]], wd, { sm: 30, seg: 28, s: [1, 1, .72] });
  // The head: thick at the eye, drawn out thin to a curved edge.
  const N = 30, rings = []; for (let i = 0; i <= N; i++) { const s = i / N, x = .0 + s * .78, hh = .16 + .5 * Math.pow(s, 1.5), yc = .68 - .1 * Math.pow(s, 2), t = .075 * Math.pow(1 - s, 1.6) + .002, bow = Math.pow(s, 6) * .12; const pts = []; for (let j = 0; j < 16; j++) { const a = j / 16 * Math.PI * 2, yy = Math.sin(a) * hh, zz = Math.cos(a) * t; pts.push(V(x + (1 - Math.pow(Math.abs(Math.sin(a)), 2)) * bow, yc + yy, zz)); } rings.push(pts); }
  const head = M(loft(rings), steel), eye = rbox(.14, .34, .16, .03, steel, { p: [0, .68, 0] }), pollH = rbox(.1, .26, .13, .03, steel, { p: [-.1, .68, 0] });
  const lang = [-1, 1].map(k => rbox(.02, .5, .05, .008, steel, { p: [0, .3, k * .037] }));
  const grip = wrap(-.7, -1.5, () => .049, hide, { turns: 30, sz: .72 }), cap = turned([[.05, -1.58], [.062, -1.62], [.056, -1.7], [0, -1.73]], steel, { sm: 24, s: [1, 1, .75] });
  return item('War axe', [part('haft', 'trim', [0, 0, 0], [haft]), part('head', 'body', [.35, .3, 0], [head, eye, pollH].concat(lang)), part('grip', 'detail', [-.3, -.2, 0], grip), part('cap', 'body', [0, -.45, 0], [cap])], { yaw: .62, fill: .96 });
}

export function hammer(c) {
  const steel = metal(c.body, { kind: 'forged', rough: .5 }), wd = wood(c.trim), cord = leather(c.detail);
  const handle = turned([[.05, -1.3], [.058, -.9], [.045, 0], [.05, .52]], wd, { sm: 40, seg: 28, s: [1, 1, .7] });
  const head = rbox(.78, .3, .3, .035, steel, { p: [0, .38, 0] }), faceA = rbox(.08, .34, .34, .03, steel, { p: [.39, .38, 0] });
  const peen = M(new THREE.CylinderGeometry(.06, .15, .26, 4, 1), steel, { r: [0, Math.PI / 4, Math.PI / 2], s: [1, 1, 1], p: [-.5, .38, 0] });
  const wedge = rbox(.07, .02, .16, .006, metal(c.body), { p: [0, .535, 0] });
  const bind = wrap(-.55, -1.2, y => .052 + (-.55 - y) * .008, cord, { turns: 26, sz: .7 });
  return item("Smith's hammer", [part('handle', 'trim', [0, -.2, 0], [handle]), part('head', 'body', [0, .4, 0], [head, faceA, peen, wedge]), part('binding', 'detail', [.3, -.3, 0], bind)], { yaw: -.6, fill: .95 });
}

export function staff(c) {
  const wd = wood(c.body), mt = metal(c.trim, { kind: 'cast' }), st = gem(c.detail), hide = leather('#3a2a22'), n = makeNoise(4, 16);
  const pts = []; for (let i = 0; i <= 40; i++) { const s = i / 40; pts.push(V((n(s * 5, 1) - .5) * .09, -2.4 + s * 4.6, (n(s * 5, 7) - .5) * .05)); }
  const shaft = M(tube(pts, s => .05 - .012 * s + .008 * Math.sin(s * 40), 14), wd);
  const top = pts[40], claws = []; for (let i = 0; i < 4; i++) claws.push(rod([[.03, 0, 0], [.14, .1, 0], [.17, .26, 0], [.11, .4, 0], [.06, .44, 0]], s => .024 * (1 - s * .5), mt, { n: 16, r: [0, i / 4 * Math.PI * 2 + .4, 0], p: [top.x, top.y - .02, top.z] }));
  const stone = M(new THREE.IcosahedronGeometry(.14, 0), st, { s: [1, 1.25, 1], p: [top.x, top.y + .22, top.z] });
  const bands = [.2, .5, .96].map(s => { const q = pts[Math.round(s * 40)]; return torus(.05 - .012 * s + .008, .014, mt, { r: [Math.PI / 2, 0, 0], p: [q.x, q.y, q.z] }); });
  const foot = turned([[.05, 0], [.055, -.1], [.03, -.22], [0, -.26]], mt, { sm: 20, p: [pts[0].x, pts[0].y, pts[0].z] });
  const g0 = pts[22], grip = wrap(g0.y - .35, g0.y + .35, () => .05, hide, { turns: 24 }); grip.forEach(m => m.geometry.translate(g0.x, 0, g0.z));
  return item('Staff', [part('shaft', 'body', [0, 0, 0], [shaft]), part('bands and foot', 'trim', [-.3, -.2, 0], bands.concat([foot])), part('grip', 'trim', [.3, 0, 0], grip), part('cradle', 'trim', [0, .3, 0], claws), part('stone', 'detail', [0, .6, 0], [stone])], { yaw: -.78, fill: 1.0 });
}

export function bow(c) {
  const wd = wood(c.body, { polish: .3 }), hide = leather(c.trim), str = matte(c.detail, { rough: .7 }), horn = bone('#d8cdb4');
  const limb = []; for (let i = 0; i <= 60; i++) { const s = i / 60 * 2 - 1, a = Math.abs(s); limb.push(V(-.62 * (1 - Math.pow(a, 2.2)) + (a > .86 ? (a - .86) * 1.3 : 0), s * 1.9, 0)); }
  const limbs = M(tube(limb, s => { const a = Math.abs(s * 2 - 1); return .055 * (1 - a * .62) + .004; }, 14), wd, { s: [1, 1, .8] });
  const gp = limb.slice(26, 35), grip = M(tube(gp, .06, 14), hide), tips = [limb[0], limb[60]];
  const nocks = tips.map(q => ball(.03, horn, { s: [1, 1.6, 1], p: [q.x, q.y, q.z] }));
  const string = rod([tips[0], tips[1]], .008, str, { sides: 6 });
  const serv = rod([[tips[0].x, -.16, 0], [tips[0].x, .16, 0]], .013, cloth('#7a2a22'), { sides: 8 });
  return item('Bow', [part('limbs', 'body', [-.25, 0, 0], [limbs]), part('grip', 'trim', [-.6, 0, 0], [grip]), part('nocks', 'trim', [0, 0, .3], nocks), part('string', 'detail', [.4, 0, 0], [string, serv])], { yaw: .5, fill: .95 });
}

export function crossbow(c) {
  const wd = wood(c.body, { polish: .3 }), iron = metal(c.trim, { kind: 'forged', rough: .5 }), str = matte(c.detail, { rough: .7 }), steel = metal('#aab3bd');
  const stock = slab([[-.09, 1.25], [.09, 1.25], [.095, .2], [.12, -.5], [.19, -1.15], [.16, -1.5], [-.16, -1.5], [-.19, -1.15], [-.12, -.5], [-.095, .2]], .2, .06, wd, { curved: true });
  const prodP = []; for (let i = 0; i <= 40; i++) { const s = i / 40 * 2 - 1; prodP.push(V(s * 1.25, 1.12 - .42 * Math.pow(Math.abs(s), 1.8), .06)); }
  const prod = M(tube(prodP, s => .05 * (1 - Math.abs(s * 2 - 1) * .55), 12), wood('#5a3a22'), { s: [1, 1, .7] });
  const string = rod([prodP[0], V(0, .12, .13), prodP[40]], .009, str, { sides: 6 });
  const stirrup = torus(.14, .022, iron, { p: [0, 1.4, 0], s: [1, 1.15, 1] });
  const lock = rbox(.1, .16, .06, .015, iron, { p: [0, .12, .12] }), trig = rod([[0, .0, -.1], [0, -.3, -.17], [0, -.62, -.15]], .018, iron, { n: 12 });
  const bind = [1.0, 1.08].map(y => torus(.105, .014, str, { r: [Math.PI / 2, 0, 0], s: [1, 1, 1.2], p: [0, y, 0] }));
  const bolt = [cyl(.016, .016, 1.0, wood('#b08a5a'), { p: [0, .68, .125] }), M(new THREE.ConeGeometry(.032, .12, 4), steel, { p: [0, 1.24, .125] })];
  return item('Crossbow', [part('stock', 'body', [0, -.3, 0], [stock]), part('prod', 'trim', [0, .35, 0], [prod].concat(bind)), part('string', 'detail', [0, .1, .3], [string]), part('stirrup', 'trim', [0, .6, 0], [stirrup]), part('lock and trigger', 'trim', [.5, -.1, 0], [lock, trig]), part('bolt', 'detail', [-.5, .2, .2], bolt)], { yaw: .55, fill: .92 });
}

export function arrows(c) {
  const hide = leather(c.body), br = metal(c.trim, { kind: 'cast' }), fl = cloth(c.detail, { sheen: .5 }), wd = wood('#b08a5a'), r = rng(8);
  const quiver = turned([[.02, -1.2], [.17, -1.18], [.2, -1.1], [.24, .5], [.25, .56], [.235, .56], [.22, .5]], hide, { sm: 0, seg: 40, s: [1, 1, .7] });
  const bands = [-.9, .44, -.2].map(y => band(.2 + (y + 1.1) * .025 + .004, y, .08, .012, br, { s: [1, 1, .7] }));
  const strap = rod([[-.2, .3, .12], [-.6, -.1, .2], [-.62, -.7, .16], [-.22, -1.0, .1]], .028, hide, { n: 30, sides: 8, s: [1, 1, .5] });
  const sh = [], fe = [];
  for (let i = 0; i < 7; i++) { const x = (r() - .5) * .3, z = (r() - .5) * .16, lean = (r() - .5) * .16, h = 1.0 + r() * .35, top = V(x + lean, h, z); sh.push(rod([[x * .6, .3, z * .6], [top.x, top.y, top.z]], .013, wd, { sides: 6 }));
    for (let k = 0; k < 3; k++) { const a = k / 3 * Math.PI * 2 + i; fe.push(sheet((u, v) => { const w = .075 * Math.sin(Math.PI * Math.pow(v, .7)) * u; return V(top.x - lean * .28 * (1 - v) + Math.cos(a) * w, top.y - .36 + v * .32, top.z + Math.sin(a) * w); }, 2, 10, fl)); } }
  return item('Quiver of arrows', [part('quiver', 'body', [0, -.3, 0], [quiver]), part('bands', 'trim', [.45, -.2, 0], bands), part('strap', 'body', [-.4, 0, 0], [strap]), part('shafts', 'trim', [0, .3, 0], sh), part('fletching', 'detail', [0, .6, 0], fe)], { yaw: .5, fill: .95 });
}

export function wand(c) {
  const wd = wood(c.body, { polish: .5 }), mt = metal(c.trim, { kind: 'cast' }), st = gem(c.detail), n = makeNoise(6, 16);
  const pts = []; for (let i = 0; i <= 30; i++) { const s = i / 30; pts.push(V((n(s * 4, 2) - .5) * .05, -.9 + s * 1.8, 0)); }
  const body = M(tube(pts, s => .05 - .028 * s + (s < .3 ? .012 * Math.sin(s / .3 * Math.PI) : 0), 14), wd);
  const bands = [.3, .36, .05].map(s => { const q = pts[Math.round(s * 30)]; return torus(.05 - .028 * s + .004 + (s < .3 ? .012 * Math.sin(s / .3 * Math.PI) : 0), .009, mt, { r: [Math.PI / 2, 0, 0], p: [q.x, q.y, q.z] }); });
  const top = pts[30], claws = []; for (let i = 0; i < 3; i++) claws.push(rod([[.016, -.03, 0], [.05, .03, 0], [.055, .1, 0], [.03, .16, 0]], s => .012 * (1 - s * .5), mt, { n: 12, r: [0, i / 3 * Math.PI * 2, 0], p: [top.x, top.y, top.z] }));
  const stone = M(new THREE.OctahedronGeometry(.05, 0), st, { s: [1, 1.5, 1], p: [top.x, top.y + .09, top.z] });
  const butt = ball(.058, mt, { p: [pts[0].x, pts[0].y - .02, 0], s: [1, .8, 1] });
  return item('Wand', [part('wand', 'body', [0, 0, 0], [body]), part('bands', 'trim', [0, -.3, 0], bands.concat([butt])), part('claw', 'trim', [0, .2, 0], claws), part('tip stone', 'detail', [0, .45, 0], [stone])], { yaw: -.7, fill: .98 });
}

export function flail(c) {
  const steel = metal(c.body, { kind: 'forged', rough: .5 }), wd = wood(c.trim), hide = leather('#2c2622'), iron = metal('#6a6e74', { kind: 'forged' });
  const handle = turned([[.05, -1.5], [.055, -1.0], [.045, -.2], [.05, -.02]], wd, { sm: 30, seg: 28 });
  const grip = wrap(-.8, -1.42, () => .053, hide, { turns: 26 });
  const cap = turned([[.05, -.06], [.064, -.02], [.06, .04], [.03, .08], [0, .08]], iron, { sm: 24 }), eye = torus(.045, .014, iron, { p: [0, .12, 0] });
  const butt = turned([[.05, -1.48], [.064, -1.52], [.05, -1.6], [0, -1.62]], iron, { sm: 24 });
  const cp = [[0, .16, 0], [.12, .42, 0], [.42, .62, 0], [.74, .56, 0], [.95, .36, 0]], ch = chain(cp, .042, iron);
  const bc = V(1.1, .16, 0), ballM = ball(.22, steel, { p: [bc.x, bc.y, bc.z] }), sp = [];
  const dirs = new THREE.IcosahedronGeometry(1, 0).attributes.position, seen = [];
  for (let i = 0; i < dirs.count; i++) { const d = V(dirs.getX(i), dirs.getY(i), dirs.getZ(i)); if (seen.some(s => s.distanceTo(d) < .01)) continue; seen.push(d); sp.push({ p: bc.clone().addScaledVector(d, .26), q: aim(d) }); }
  const spikes = many(new THREE.ConeGeometry(.055, .2, 6), steel, sp);
  return item('Flail', [part('handle', 'trim', [0, -.2, 0], [handle]), part('grip', 'trim', [-.3, -.3, 0], grip), part('cap and ring', 'trim', [0, .1, 0], [cap, eye, butt]), part('chain', 'trim', [.1, .3, 0], [ch]), part('ball', 'body', [.5, .3, 0], [ballM, spikes])], { yaw: .35, fill: .95 });
}
