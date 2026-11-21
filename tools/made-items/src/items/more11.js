// Materials and creature parts: planks, coal, salt, glass, beeswax, silk cocoons, an ice shard, a pearl, bones, a stinger, an antler, a jar of slime, a flower and bottled lightning.
import { THREE, V, M, part, item, turned, smooth, lathe, rod, ball, torus, cyl, rbox, shape, blob, sheet, many, aim, band, fan, crystalGeo, strands, rng } from '../kit.js';
import { metal, wood, leather, cloth, matte, bone, rock, gem, liquid, glow, ghost, dark, shell, glass as glassOf, wax as waxOf } from '../mats.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
const sstep = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
const fine = (m, n) => { for (const k of ['map', 'bumpMap']) if (m[k]) { m[k] = m[k].clone(); m[k].repeat.set(n, n); m[k].needsUpdate = true; } return m; };
// A broken lump: a coarse ball pushed about, with flat faces.
const lump = (R, sd, mat, o) => { const g = new THREE.IcosahedronGeometry(R, 1), p = g.attributes.position; for (let i = 0; i < p.count; i++) { const x = p.getX(i) / R, y = p.getY(i) / R, z = p.getZ(i) / R, k = 1 + .2 * Math.sin(x * 5 + sd) + .16 * Math.sin(y * 6 + sd * 1.7) + .14 * Math.sin(z * 7 + sd * .6); p.setXYZ(i, x * R * k, y * R * k * .85, z * R * k); } g.computeVertexNormals(); return M(g, mat, o); };

export function planks(c) {
  const wd = wood(c.body, { bump: 1.1 }), rope = matte(c.trim, { rough: .95, bump: .9 }), paint = matte(c.detail, { rough: .7 }), L = 2.2, Wb = .42, Tb = .1, boards = [], ends = [];
  // Five sawn boards, stacked and roped; the merchant has painted the ends.
  for (const [z, y, x, yaw] of [[-.22, .05, 0, .02], [.22, .05, .04, -.03], [-.2, .152, .05, -.02], [.23, .152, -.06, .03], [.02, .254, 0, .1]]) {
    boards.push(rbox(L, Tb, Wb, .012, wd, { r: [0, yaw, 0], p: [x, y, z] }));
    for (const k of [-1, 1]) ends.push(rbox(.008, Tb * .82, Wb * .92, .002, paint, { r: [0, yaw, 0], p: [x + k * (L / 2 + .002) * Math.cos(yaw), y, z - k * (L / 2 + .002) * Math.sin(yaw)] }));
  }
  const ropes = [-.62, .62].map(x => [rod([[x, 0, -.47], [x, .29, -.47], [x, .325, -.42], [x, .325, .42], [x, .29, .47], [x, 0, .47]], .024, rope, { sides: 8 }), ball(.05, rope, { p: [x, .34, .1] }), rod([[x, .34, .1], [x + .1, .34, .24], [x + .14, .31, .4]], .02, rope, { n: 8, sides: 6 })]).flat();
  return item('Planks', [part('boards', 'body', [0, 0, 0], boards), part('rope', 'trim', [0, .5, 0], ropes), part('painted ends', 'detail', [.8, 0, 0], ends)], { stand: true, yaw: .62, el: 22 });
}

export function coal(c, t) {
  const ph = (t || 0) * TAU, black = matte(c.body, { rough: .34, wet: .35, flat: true }), iron = metal(c.trim, { kind: 'forged', rough: .55, double: true }), r = rng(17), lumps = [], embers = [];
  // A heap in an iron pan, still alight underneath.
  const pan = turned(smooth([[0, .0], [.9, .0], [1.06, .1], [1.1, .16]], 20).concat(smooth([[1.06, .15], [.9, .05], [0, .05]], 20)), iron, { sm: 0 });
  for (const [x, y, z, R] of [[-.5, .2, .2, .3], [.05, .22, .42, .33], [.55, .2, .1, .3], [.3, .2, -.42, .32], [-.3, .22, -.38, .34], [-.12, .5, .05, .33], [.3, .5, .0, .28], [-.02, .5, -.3, .26], [.1, .74, -.06, .24]]) lumps.push(lump(R, r() * 9, black, { r: [r() * 3, r() * 3, 0], p: [x, y, z] }));
  [[-.2, .3, .3], [.3, .3, .26], [.1, .32, -.1], [-.36, .34, -.08], [.34, .36, -.22], [.02, .56, .2]].forEach(([x, y, z], i) => embers.push(lump(.15, i * 2.3, glow(c.detail, 1.5 + .55 * Math.sin(ph + i * 1.9)), { p: [x, y, z] })));
  const sparks = Array.from({ length: 7 }, (_, i) => { const h = ((t || 0) + i / 7) % 1, a = i * 2.4; return ball(.016 * (1 - h), glow(c.detail, 2.6), { p: [Math.cos(a) * .3 + Math.sin(h * 6 + i) * .06, .6 + h * .9, Math.sin(a) * .25], seg: 8, seg2: 6, nofit: true }); });
  return item('Coal', [part('coal', 'body', [0, .3, 0], lumps), part('pan', 'trim', [0, -.3, 0], [pan]), part('embers', 'detail', [0, .5, .4], embers.concat(sparks))], { stand: true, yaw: .2, el: 22, lights: [{ p: [0, .42, .15], color: c.detail, power: .8 + .2 * Math.sin(ph) }], halo: .8 });
}

export function salt(c) {
  const grainM = matte(c.body, { rough: .55, grain: 'rock', bump: .6 }), crystalM = matte(c.body, { rough: .2, wet: .6 }), wd = wood(c.trim, { polish: .3 }), horn = bone(c.detail, { gloss: .6 }), r = rng(9), bits = [];
  const bowl = turned(smooth([[0, 0], [.5, 0], [.58, .05], [.92, .38], [.96, .5]], 30).concat(smooth([[.92, .5], [.84, .4], [.5, .1], [0, .08]], 30)), wd, { sm: 0 });
  // Coarse salt heaped past the rim, with the biggest crystals lying on top.
  const top = v => .42 + .3 * Math.pow(Math.max(0, 1 - v * v), .9), heap = sheet((u, v) => { const a = u * TAU, d = v * .8; return V(Math.cos(a) * d, top(v) + .02 * Math.sin(a * 5 + v * 9) * v, Math.sin(a) * d); }, 48, 18, matte(c.body, { rough: .55, grain: 'rock', bump: .6, double: true }));
  for (let i = 0; i < 90; i++) { const a = r() * TAU, v = Math.sqrt(r()) * .95, k = .5 + r() * .9; bits.push({ p: V(Math.cos(a) * v * .8, top(v) + .012, Math.sin(a) * v * .8), r: [r() * 3, r() * 3, r() * 3], s: k }); }
  const cubes = many(new THREE.BoxGeometry(.06, .06, .06), crystalM, bits), spilled = many(new THREE.BoxGeometry(.05, .05, .05), grainM, Array.from({ length: 14 }, () => { const a = .6 + r() * 1.6, d = 1.05 + r() * .3; return { p: V(Math.cos(a) * d, .02, Math.sin(a) * d), r: [r() * 3, r() * 3, 0], s: .5 + r() * .7 }; }));
  // A horn spoon stands in it.
  const spoon = [rod([[.2, .52, .1], [.5, .9, .2], [.74, 1.3, .3]], s => .04 - .012 * s, horn, { n: 16, sides: 10 }), blob([.13, .05, .17], null, horn, { r: [.5, 0, -.6], p: [.17, .5, .09] })];
  return item('Salt', [part('salt', 'body', [0, .3, 0], [heap, cubes, spilled]), part('bowl', 'trim', [0, -.3, 0], [bowl]), part('spoon', 'detail', [.5, .5, .2], spoon)], { stand: true, yaw: .2, el: 24 });
}

// The part of a polygon on the inner side of one edge of a box.
const clip = (pts, ax, lim, sign) => { const out = []; for (let i = 0; i < pts.length; i++) { const a = pts[i], b = pts[(i + 1) % pts.length], ia = sign * (a[ax] - lim) <= 0, ib = sign * (b[ax] - lim) <= 0; if (ia) out.push(a); if (ia !== ib) { const k = (lim - a[ax]) / (b[ax] - a[ax]); out.push([a[0] + (b[0] - a[0]) * k, a[1] + (b[1] - a[1]) * k]); } } return out; };
export function glass(c) {
  const clear = glassOf(c.body, { tint: .5 }), stained = glassOf(c.detail, { tint: .62 }), lead = metal(c.trim, { kind: 'cast', rough: .55 }), W = .6, H = .8, A = [], B = [], cames = [];
  // A leaded light: lozenges of plain glass with coloured ones set between them.
  const pane = (cx, cy) => { let q = [[cx, cy + .185], [cx + .185, cy], [cx, cy - .185], [cx - .185, cy]]; q = clip(clip(clip(clip(q, 0, W - .03, 1), 0, -W + .03, -1), 1, H - .03, 1), 1, -H + .03, -1); return q.length > 2 ? shape(q) : null; };
  for (let i = -1; i <= 1; i++) for (let j = -2; j < 2; j++) A.push(pane(i * .4, j * .4 + .2));
  for (let i = -2; i < 2; i++) for (let j = -2; j <= 2; j++) B.push(pane(i * .4 + .2, j * .4));
  const sheetOf = (list, mat) => M(new THREE.ShapeGeometry(list.filter(Boolean)), mat);
  for (let k = -3; k <= 3; k++) for (const s of [1, -1]) { const cc = .4 * k, x0 = Math.max(-W, s > 0 ? cc - H : -H - cc), x1 = Math.min(W, s > 0 ? cc + H : H - cc); if (x1 - x0 > .02) cames.push(rod([[x0, s > 0 ? cc - x0 : x0 + cc, 0], [x1, s > 0 ? cc - x1 : x1 + cc, 0]], .017, lead, { sides: 8, s: [1, 1, .7] })); }
  const frame = [rbox(2 * W + .08, .07, .06, .015, lead, { p: [0, H, 0] }), rbox(2 * W + .08, .07, .06, .015, lead, { p: [0, -H, 0] }), rbox(.07, 2 * H + .08, .06, .015, lead, { p: [W, 0, 0] }), rbox(.07, 2 * H + .08, .06, .015, lead, { p: [-W, 0, 0] })];
  return item('Glass', [part('plain glass', 'body', [0, 0, 0], [sheetOf(A, clear)]), part('lead', 'trim', [0, 0, .5], cames.concat(frame)), part('stained glass', 'detail', [0, 0, -.5], [sheetOf(B, stained)])], { fill: .93 });
}

export function wax(c) {
  const wx = waxOf(c.body, { lit: .1 }), wd = wood(c.trim, { bump: .9 }), twine = matte(c.detail, { rough: .95, bump: .9 }), cells = [];
  // Three cakes as they came from the mould, the top one still showing the comb.
  const cake = (y, x, z) => turned([[0, 0], [.5, 0], [.56, .04], [.57, .2], [.52, .25], [0, .25]], wx, { sm: 40, p: [x, y, z] });
  const cakes = [cake(.06, 0, 0), cake(.31, .03, -.02), cake(.56, -.02, .02)];
  for (let i = -4; i <= 4; i++) for (let j = -4; j <= 4; j++) { const x = (i + (j % 2 ? .5 : 0)) * .112, z = j * .097; if (x * x + z * z < .16) cells.push({ p: V(x - .02, .812, z + .02) }); }
  const comb = many(new THREE.CylinderGeometry(.058, .062, .02, 6), wx, cells), board = rbox(1.7, .06, 1.3, .015, wd, { p: [0, .03, 0] });
  const loop = (ry) => rod([[-.585, .06, 0], [-.585, .78, 0], [-.54, .822, 0], [.54, .822, 0], [.585, .78, 0], [.585, .06, 0]], .016, twine, { sides: 6, r: [0, ry, 0] });
  const bow = [torus(.07, .014, twine, { r: [H2, 0, 0], s: [1.6, 1, 1], p: [.1, .84, 0] }), torus(.07, .014, twine, { r: [H2, 0, 0], s: [1.6, 1, 1], p: [-.1, .84, 0] }), ball(.03, twine, { p: [0, .84, 0] }), rod([[0, .84, 0], [.1, .83, .16], [.2, .825, .3]], .013, twine, { n: 8, sides: 6 })];
  return item('Beeswax', [part('wax', 'body', [0, 0, 0], cakes.concat([comb])), part('board', 'trim', [0, -.3, 0], [board]), part('twine', 'detail', [0, .5, .3], [loop(0), loop(H2)].concat(bow))], { stand: true, yaw: .35, el: 26 });
}

export function cocoon(c) {
  const silk = fine(cloth(c.body, { sheen: 1, bump: .7 }), 4), bark = wood(c.trim, { bump: 1.4 }), leafM = matte(c.detail, { rough: .6, double: true, sheen: .3 }), threadM = matte(c.body, { rough: .6 }), r = rng(12);
  const twig = rod([[-1.1, -.5, 0], [-.5, -.1, .04], [.1, .2, 0], [.7, .62, -.03], [1.15, .82, 0]], s => .06 - .03 * s, bark, { n: 40, sides: 10 }), fork = rod([[.1, .2, 0], [.3, .62, .06], [.36, 1.0, .1]], s => .035 - .02 * s, bark, { n: 16, sides: 8 });
  const coc = [], threads = [];
  // Each cocoon hangs under the twig in a tangle of its own thread.
  for (const [x, y, z, tilt, k] of [[-.62, -.6, .08, .25, 1], [.0, -.3, .1, -.1, 1.12], [.62, .12, .06, .2, .94]]) {
    const o = { r: [0, 0, tilt], p: [x, y, z] };
    coc.push(blob([.2 * k, .33 * k, .2 * k], v => 1 - .06 * Math.exp(-Math.pow(v.y / .25, 2)), silk, o));
    const hel = []; for (let i = 0; i <= 90; i++) { const s = i / 90, yy = (s * 2 - 1) * .31 * k, rr = .205 * k * Math.sqrt(Math.max(0, 1 - Math.pow(yy / (.33 * k), 2))) + .004, a = s * TAU * 7; hel.push(V(Math.cos(a) * rr, yy, Math.sin(a) * rr)); }
    threads.push(rod(hel, .005, threadM, Object.assign({ sides: 5 }, o)));
    const topX = x - Math.sin(tilt) * .33 * k, topY = y + Math.cos(tilt) * .33 * k;
    for (let i = 0; i < 7; i++) threads.push(rod([[topX + (r() - .5) * .1, topY - .06, z + (r() - .5) * .1], [topX + (r() - .5) * .2, topY + .14, z * .6], [topX + (r() - .5) * .4, topY + .26 + r() * .08, .02]], .004, threadM, { n: 8, sides: 4 }));
  }
  const lo = [[-.16, 0], [-.1, .16], [.08, .24], [.3, .2], [.5, .1], [.7, 0], [.5, -.1], [.3, -.2], [.08, -.24], [-.1, -.16]];
  const lr = [.3, .2, .9], lp = [.4, 1.06, .1], leaf = fan(lo, (x, y) => .07 * Math.sin(x * 4) - .5 * y * y, leafM, { nu: 72, nv: 8, cx: .2, tension: .3, r: lr, p: lp }), rib = rod([[-.16, 0, 0], [.25, 0, .06], [.68, 0, .03]], .008, leafM, { n: 8, sides: 5, r: lr, p: lp });
  return item('Silk cocoon', [part('cocoons', 'body', [0, -.3, 0], coc.concat(threads)), part('twig', 'trim', [0, .4, 0], [twig, fork]), part('leaf', 'detail', [.4, .6, 0], [leaf, rib])], { fill: .95 });
}

export function ice(c) {
  const frost = gem(c.body, { vc: true, frost: .16, pale: .5, body: .22 }), snow = matte(c.trim, { rough: .95, sheen: .6, grain: 'rock', bump: .5 }), n = rng(6);
  // One long splinter fallen across the snow, smaller ones broken off round it.
  const big = M(crystalGeo(.3, 2.3, .9, { seed: 4, sides: 5, taper: .5, hex: c.body }), frost, { r: [0, .3, -1.02], p: [-.9, .16, 0] });
  const rest = [[.2, .5, .16, .9, .5, -.3], [-.5, .3, .11, .62, -.4, .5], [.62, -.2, .09, .5, -.2, -.9], [-.2, -.42, .07, .4, .8, .3]].map(([x, z, R, len, rx, rz], i) => M(crystalGeo(R, len, len * .45, { seed: i + 11, sides: 5, taper: .5, hex: c.body }), frost, { r: [rx, 0, rz], p: [x, .1, z] }));
  const drift = blob([1.35, .16, .8], v => 1 + .14 * Math.sin(v.x * 6 + 1) * Math.sin(v.z * 5), snow, { p: [0, .02, 0] });
  const chips = many(new THREE.OctahedronGeometry(.035, 0), matte(c.trim, { rough: .3, wet: .6 }), Array.from({ length: 26 }, () => { const a = n() * TAU, d = .3 + n() * .9; return { p: V(Math.cos(a) * d * 1.1, .13 + .03 * n(), Math.sin(a) * d * .6), r: [n() * 3, n() * 3, 0], s: .5 + n() }; }));
  // Something cold still burns in the heart of it.
  const heart = [rod([[-.62, .32, .0], [-.2, .58, .0], [.3, .86, 0]], s => .04 * Math.sin(Math.PI * (.1 + s * .9)), glow(c.detail, 1.6), { n: 12, sides: 8 }), rod([[-.7, .28, .0], [-.2, .58, .0], [.4, .92, 0]], s => .1 * Math.sin(Math.PI * s), ghost(c.detail, .1), { n: 12, sides: 8 })];
  return item('Ice shard', [part('ice', 'body', [0, .3, 0], [big].concat(rest)), part('snow', 'trim', [0, -.3, 0], [drift, chips]), part('cold light', 'detail', [0, .4, .5], heart)], { stand: true, yaw: .15, el: 18, lights: [{ p: [-.2, .6, .2], color: c.detail, power: .5 }], halo: .7 });
}

export function pearl(c) {
  const lustre = matte(c.body, { rough: .1, wet: 1, sheen: 1 }), rough = rock(c.trim, { rough: .8, bump: 2 }), nacre = shell(c.detail, { rough: .16, double: true, bump: .2 }), flesh = matte('#d8b8a8', { rough: .35, wet: .8 });
  rough.side = THREE.DoubleSide;
  // Both halves of the shell, the upper thrown back on its hinge; the pearl sits in the lower.
  const rim = []; for (let i = 0; i < 48; i++) { const a = i / 48 * TAU, rr = .86 * (1 + .035 * Math.cos(a * 13)) * (1 - .1 * Math.max(0, Math.sin(a))); rim.push([Math.cos(a) * rr * 1.12, Math.sin(a) * rr]); }
  const dish = (k, off) => (x, y, v) => k * (-.3 * (1 - v * v) + .012 * Math.cos(Math.atan2(y, x) * 13) * v) + off;
  const lowerIn = fan(rim, dish(1, 0), nacre, { nu: 96, nv: 20, r: [-H2, 0, 0], p: [0, .32, 0] }), lowerOut = fan(rim, dish(1.05, -.03), rough, { nu: 96, nv: 20, r: [-H2, 0, 0], p: [0, .32, 0] });
  const lid = (mat, k, off) => { const m = fan(rim, dish(k, off), mat, { nu: 96, nv: 20, r: [-H2, 0, 0] }); m.geometry.translate(0, 0, .78); m.geometry.rotateX(-1.45); m.geometry.translate(0, .34, -.78); return m; };
  const upperIn = lid(nacre, -1, 0), upperOut = lid(rough, -1.05, .03);
  const hinge = blob([.3, .08, .1], null, rough, { p: [0, .32, -.84] });
  const bed = blob([.5, .1, .42], v => 1 + .08 * Math.sin(v.x * 9) * Math.sin(v.z * 7), flesh, { p: [0, .1, .02] }), gemM = ball(.21, lustre, { p: [.06, .31, .1] });
  return item('Pearl', [part('pearl', 'body', [0, .5, .3], [gemM]), part('shell', 'trim', [0, -.3, 0], [lowerOut, upperOut, hinge]), part('mother of pearl', 'detail', [0, .3, -.3], [lowerIn, upperIn]), part('oyster', '', [0, 0, .5], [bed])], { stand: true, yaw: .3, el: 26 });
}

export function bones(c) {
  const bn = bone(c.body, { rough: .5 }), thong = leather(c.trim, { bump: .9 }), paint = matte(c.detail, { rough: .6 }), all = [], marks = [];
  // Three long bones tied in a bundle, each ringed with paint.
  const P = [[0, -1.0], [.1, -.99], [.16, -.9], [.13, -.76], [.075, -.5], [.062, 0], [.075, .5], [.13, .76], [.16, .9], [.1, .99], [0, 1.0]];
  for (const [x, z, tilt, k] of [[-.19, 0, .13, 1], [.02, .12, -.03, 1.08], [.2, -.02, -.15, .94]]) {
    const o = { s: k, r: [0, 0, tilt], p: [x, 0, z] };
    all.push(turned(P, bn, Object.assign({ sm: 60 }, o)));
    for (const e of [-1, 1]) for (const d of [-1, 1]) all.push(ball(.095, bn, { s: [k, k * 1.1, k], r: [0, 0, tilt], p: [x - Math.sin(tilt) * e * .93 * k + d * .075 * k * Math.cos(tilt), e * .93 * k * Math.cos(tilt) + d * .075 * k * Math.sin(tilt), z] }));
    for (const y of [-.36, .3, .38]) marks.push(band(.071, y, .04, .008, paint, Object.assign({ seg: 24 }, o)));
  }
  const wraps = [-.09, -.03, .03, .09].map(y => torus(.33, .03, thong, { s: [1, .62, 1], r: [H2, 0, 0], p: [.01, y, .04] })), knot = [ball(.06, thong, { p: [.05, 0, .28] }), rod([[.05, 0, .28], [.2, -.2, .32], [.26, -.5, .28]], .022, thong, { n: 10, sides: 6 }), rod([[.05, 0, .28], [-.06, -.24, .34], [-.02, -.44, .3]], .022, thong, { n: 10, sides: 6 })];
  return item('Bones', [part('bones', 'body', [0, 0, 0], all), part('thong', 'trim', [0, 0, .6], wraps.concat(knot)), part('paint', 'detail', [0, .6, .3], marks)], { yaw: .2, fill: .95 });
}

export function stinger(c) {
  const chitin = matte(c.body, { rough: .16, wet: 1 }), sacM = matte(c.trim, { rough: .3, wet: .8, sheen: .4 }), venom = liquid(c.detail, { lit: .6, inner: 1.4 }), raw = matte('#7a2a26', { rough: .5, wet: .5 });
  // The venom sac and the barb it feeds, torn from whatever carried it.
  const sac = blob([.42, .5, .4], v => 1 + .05 * Math.sin(Math.atan2(v.z, v.x) * 6) * (1 - v.y * v.y), sacM, { p: [-.3, .72, 0], r: [0, 0, .3] });
  const veins = [0, 1, 2, 3, 4].map(i => { const a = i * 1.26 + .3, pts = []; for (let j = 0; j <= 10; j++) { const th = .5 + j / 10 * 2.0, d = V(Math.sin(th) * Math.cos(a + Math.sin(j) * .2), Math.cos(th), Math.sin(th) * Math.sin(a + Math.sin(j) * .2)); pts.push(V(d.x * .43, d.y * .51, d.z * .41)); } return rod(pts, .014, chitin, { sides: 6, r: [0, 0, .3], p: [-.3, .72, 0] }); });
  const path = [[-.18, .34, 0], [-.08, -.1, 0], [.06, -.56, 0], [.3, -.96, 0], [.64, -1.22, 0], [1.02, -1.32, 0]];
  const barb = rod(path, s => .2 * Math.pow(1 - s, 1.25) + .003, chitin, { n: 60, sides: 18 });
  const rings = [[-.17, .3, .2, .3], [-.12, .08, .19, .24], [-.06, -.14, .17, .2]].map(([x, y, R, tilt]) => torus(R, .028, chitin, { r: [H2, 0, tilt], p: [x, y, 0] }));
  const stump = blob([.2, .08, .2], null, raw, { r: [0, 0, .3], p: [-.45, 1.2, 0] });
  const drop = [blob([.05, .08, .05], v => 1 + .4 * Math.max(0, v.y), venom, { p: [1.06, -1.42, 0] }), ball(.026, glow(c.detail, 1.6), { p: [1.02, -1.325, 0], seg: 10, seg2: 8 })];
  return item('Stinger', [part('barb', 'body', [.3, -.3, 0], [barb].concat(rings, veins)), part('venom sac', 'trim', [-.3, .4, 0], [sac]), part('venom', 'detail', [.6, -.5, .3], drop), part('torn end', '', [-.4, .8, 0], [stump])], { yaw: .15, fill: .95, lights: [{ p: [1.0, -1.3, .3], color: c.detail, power: .3 }] });
}

export function antler(c) {
  const horn = bone(c.body, { rough: .6, gloss: .2 }), strap = leather(c.trim, { bump: .9 }), beadM = matte(c.detail, { rough: .3, wet: .6 }), r = rng(23);
  // One side, shed whole: the beam, four tines and the rough crown where it grew.
  const beamP = [[-.62, -1.25, 0], [-.56, -.7, .05], [-.34, -.1, .1], [.04, .42, .06], [.42, .82, 0], [.7, 1.3, -.05]], beam = rod(beamP, s => .135 - .09 * s, horn, { n: 60, sides: 16 });
  const tines = [[[-.58, -.9, .04], [-.2, -.86, .3], [.1, -.6, .46], [.2, -.3, .5]], [[-.44, -.36, .08], [-.1, -.2, .2], [.1, .1, .22], [.12, .42, .16]], [[.0, .38, .06], [.36, .34, .1], [.62, .5, .08], [.76, .74, .02]], [[.4, .8, 0], [.3, 1.1, -.02], [.36, 1.4, -.06]]].map(p => rod(p, s => .09 * Math.pow(1 - s, .8) + .008, horn, { n: 30, sides: 12 }));
  const crown = many(new THREE.SphereGeometry(.05, 10, 8), horn, Array.from({ length: 16 }, (_, i) => { const a = i / 16 * TAU; return { p: V(-.62 + Math.cos(a) * .17, -1.2 + (r() - .5) * .04, Math.sin(a) * .17), s: .7 + r() * .7 }; })), base = cyl(.14, .16, .1, horn, { p: [-.62, -1.27, 0] });
  // A carrying strap with three beads on its tail.
  const wraps = [0, 1, 2, 3].map(i => torus(.122 - i * .003, .024, strap, { r: [H2, 0, -.1], p: [-.585 + i * .008, -.96 + i * .05, .03] })), tail = rod([[-.5, -.9, .12], [-.4, -1.1, .22], [-.36, -1.4, .2], [-.4, -1.62, .16]], .016, strap, { n: 14, sides: 6 });
  const beads = [[-.37, -1.36, .2], [-.375, -1.47, .19], [-.39, -1.58, .17]].map((p, i) => ball(.05 - i * .004, beadM, { p: p }));
  return item('Antler', [part('antler', 'body', [0, 0, 0], [beam, crown, base].concat(tines)), part('strap', 'trim', [-.3, -.4, .3], wraps.concat([tail])), part('beads', 'detail', [-.2, -.7, .5], beads)], { yaw: .25, fill: .95 });
}

export function slime(c, t) {
  const ph = (t || 0) * TAU, gl = glassOf(c.body, { tint: .22 }), cover = fine(cloth(c.trim, { bump: 1, sheen: .3 }), 2), cord = matte('#5a4630', { rough: .95, bump: .8 }), ooze = liquid(c.detail, { lit: .5 + .1 * Math.sin(ph), inner: 1.7 }), r = rng(41);
  const jar = turned([[0, .03], [.5, .03], [.56, .1], [.57, .95], [.46, 1.1], [.4, 1.14], [.4, 1.26], [.44, 1.28]], gl, { sm: 60 });
  // It never quite lies still: the top heaves, and it has climbed the glass on one side.
  const top = (x, z) => .74 + .05 * Math.sin(x * 5 + ph) * Math.cos(z * 4 - ph) + .2 * Math.exp(-(Math.pow(x - .34, 2) + z * z) / .05);
  const fill = turned([[0, .06], [.48, .06], [.53, .12], [.54, .7]], ooze, { sm: 20 }), surf = sheet((u, v) => { const a = u * TAU, d = v * .54, x = Math.cos(a) * d, z = Math.sin(a) * d; return V(x, top(x, z) * (.94 + .06 * (1 - v)), z); }, 40, 14, ooze);
  const bubbles = Array.from({ length: 10 }, () => { const a = r() * TAU, d = r() * .4, h = (r() + (t || 0)) % 1; return ball(.02 + r() * .025, glow(c.detail, 2 * Math.min(1, h * 5) * (1 - h * .6), { add: true }), { p: [Math.cos(a) * d, .12 + h * .56, Math.sin(a) * d], seg: 10, seg2: 8 }); });
  const lid = lathe(smooth([[0, 1.33], [.3, 1.34], [.45, 1.3], [.47, 1.2], [.5, 1.08], [.6, 1.0]], 30), 64), lp = lid.attributes.position;
  for (let i = 0; i < lp.count; i++) { const x = lp.getX(i), y = lp.getY(i), z = lp.getZ(i), k = 1 + .1 * Math.sin(Math.atan2(z, x) * 11) * sstep(1.2, 1.0, y); lp.setXYZ(i, x * k, y - .03 * Math.sin(Math.atan2(z, x) * 7) * sstep(1.15, 1.0, y), z * k); }
  lid.computeVertexNormals(); const lidM = M(lid, cover), tie = [torus(.455, .02, cord, { r: [H2, 0, 0], p: [0, 1.2, 0] }), ball(.035, cord, { p: [0, 1.2, .47] }), rod([[0, 1.2, .47], [.1, 1.06, .56], [.14, .9, .58]], .014, cord, { n: 8, sides: 6 })];
  // A drip has got out under the cloth and runs down the outside.
  const drip = [rod([[-.3, 1.02, .5], [-.3, .8, .5], [-.29, .56, .5]], s => .03 + .02 * Math.sin(s * Math.PI) + .03 * sstep(.7, 1, s), ooze, { n: 16, sides: 10 }), blob([.16, .014, .12], null, ooze, { p: [-.34, .012, .74] })];
  return item('Jar of slime', [part('jar', 'body', [0, 0, 0], [jar]), part('cover', 'trim', [0, .6, 0], [lidM]), part('slime', 'detail', [0, 0, .9], [fill, surf].concat(bubbles, drip)), part('cord', '', [0, .5, .4], tie)], { stand: true, yaw: .2, el: 14, lights: [{ p: [0, .45, 0], color: c.detail, power: .8 }], halo: .6 });
}

export function flower(c) {
  const petalM = matte(c.body, { rough: .5, double: true, sheen: .8 }), green = matte(c.trim, { rough: .6, double: true, sheen: .3 }), gold = matte(c.detail, { rough: .5 }), Y0 = .62, bloom = [], heart = [];
  // Three rings of petals, the outer ones fallen open.
  const petal = (phi, tilt, len, wid, curl, lift) => sheet((u, v) => { const e = 2 * u - 1, w = wid * Math.pow(Math.sin(Math.PI * (.08 + .62 * Math.pow(v, .6))), .6), x = (u - .5) * w, l = v * len * (1 - .24 * e * e), tt = tilt + curl * v * v, rr = .05 + l * Math.sin(tt) - 1.1 * x * x * (1 - v * .4), y = Y0 + lift + l * Math.cos(tt) + .4 * x * x; return V(Math.cos(phi) * rr - Math.sin(phi) * x, y, Math.sin(phi) * rr + Math.cos(phi) * x); }, 14, 16, petalM);
  for (let i = 0; i < 7; i++) bloom.push(petal(i / 7 * TAU, .95, .86, .78, .6, 0));
  for (let i = 0; i < 6; i++) bloom.push(petal((i + .5) / 6 * TAU, .58, .74, .66, .5, .03));
  for (let i = 0; i < 5; i++) bloom.push(petal((i + .2) / 5 * TAU, .28, .58, .5, .3, .05));
  heart.push(ball(.09, gold, { p: [0, Y0 + .14, 0], s: [1, .8, 1] }));
  for (let i = 0; i < 9; i++) { const a = i / 9 * TAU; heart.push(rod([[0, Y0 + .1, 0], [Math.cos(a) * .08, Y0 + .26, Math.sin(a) * .08], [Math.cos(a) * .15, Y0 + .36, Math.sin(a) * .15]], .008, gold, { n: 6, sides: 5 }), ball(.022, gold, { p: [Math.cos(a) * .15, Y0 + .37, Math.sin(a) * .15], seg: 10, seg2: 8 })); }
  const cup = turned([[.03, Y0 - .16], [.07, Y0 - .1], [.11, Y0], [.09, Y0 + .06]], green, { sm: 16 }), sepals = [0, 1, 2, 3, 4].map(i => { const a = (i + .3) / 5 * TAU; return rod([[Math.cos(a) * .08, Y0 - .02, Math.sin(a) * .08], [Math.cos(a) * .2, Y0 + .0, Math.sin(a) * .2], [Math.cos(a) * .3, Y0 - .1, Math.sin(a) * .3]], s => .03 * (1 - s) + .004, green, { n: 8, sides: 6 }); });
  // The head nods toward the eye on its stem.
  const nod = m => { m.geometry.translate(0, -Y0 + .16, 0); m.geometry.rotateX(1.15); m.geometry.translate(0, Y0 - .16, 0); return m; };
  bloom.concat(heart, sepals, [cup]).forEach(nod);
  const stem = rod([[.1, -1.7, 0], [.02, -1.0, .04], [-.04, -.3, 0], [0, Y0 - .16, 0]], s => .034 - .012 * s, green, { n: 40, sides: 10 });
  const leafO = [[0, 0], [.14, .13], [.38, .16], [.66, 0], [.38, -.16], [.14, -.13]], lz = (x, y) => .12 * Math.sin(x * 3.4) - 1.6 * y * y;
  const leaves = [fan(leafO, lz, green, { nu: 48, nv: 6, cx: .3, r: [.5, 0, .5], p: [0, -.82, .03] }), fan(leafO, lz, green, { nu: 48, nv: 6, cx: .3, s: .86, r: [.5, Math.PI, .6], p: [-.02, -.36, .02] })];
  return item('Flower', [part('petals', 'body', [0, .5, 0], bloom), part('stem and leaves', 'trim', [0, -.5, 0], [stem, cup].concat(sepals, leaves)), part('heart', 'detail', [0, .6, .6], heart)], { yaw: .1, fill: .95 });
}

export function lightning(c, t) {
  const fr = Math.floor((t || 0) * 16), r = rng(100 + fr), gl = glassOf(c.body, { tint: .2 }), cu = metal(c.trim, { kind: 'cast', rough: .3 }), corkM = matte('#a8845a', { rough: .9, grain: 'rock', bump: .8 }), power = .7 + r() * .7;
  const bottle = turned([[0, .03], [.46, .03], [.52, .1], [.53, .9], [.44, 1.14], [.2, 1.32], [.17, 1.56], [.21, 1.6]], gl, { sm: 60 });
  // A rod runs down through the cork, and the storm will not let go of it.
  const cork = cyl(.17, .2, .2, corkM, { p: [0, 1.62, 0] }), cap = turned([[0, 1.9], [.05, 1.9], [.08, 1.84], [.06, 1.74], [.19, 1.73], [.21, 1.7]], cu, { sm: 0, seg: 28 }), spike = rod([[0, 1.9, 0], [0, 2.14, 0]], s => .03 * (1 - s) + .004, cu, { sides: 8 }), down = rod([[0, 1.7, 0], [0, .98, 0]], .022, cu, { sides: 8 }), knob = ball(.07, cu, { p: [0, .96, 0] });
  const cage = [0, 1, 2, 3].map(i => { const a = i * H2 + .4, x = Math.cos(a), z = Math.sin(a); return rod([[x * .2, 1.7, z * .2], [x * .22, 1.5, z * .22], [x * .46, 1.16, z * .46], [x * .55, .9, z * .55], [x * .55, .12, z * .55], [x * .46, .02, z * .46]], .016, cu, { n: 40, sides: 6 }); }).concat([torus(.552, .018, cu, { r: [H2, 0, 0], p: [0, .5, 0] }), torus(.5, .018, cu, { r: [H2, 0, 0], p: [0, .03, 0] })]);
  // Each flash finds its own way from the knob to the glass.
  const bolts = [], strike = (from, to, n, w) => { const pts = [from]; for (let i = 1; i < n; i++) { const s = i / n; pts.push(from.clone().lerp(to, s).add(V((r() - .5) * .22, (r() - .5) * .12, (r() - .5) * .22))); } pts.push(to); bolts.push(rod(pts, w, glow(c.detail, 3.2 * power), { sides: 5 }), rod(pts, w * 4.5, ghost(c.detail, .16 * power), { sides: 6 })); return pts; };
  const a0 = r() * TAU, main = strike(V(0, .9, 0), V(Math.cos(a0) * .44, .12 + r() * .3, Math.sin(a0) * .44), 7, .014), a1 = a0 + 2 + r() * 2;
  strike(main[3], V(Math.cos(a1) * .47, .3 + r() * .4, Math.sin(a1) * .47), 4, .009); strike(main[5], V(Math.cos(a0 + .9) * .42, .08, Math.sin(a0 + .9) * .42), 3, .007);
  const cloud = ball(.3, ghost(c.detail, .05 + .03 * power), { p: [0, .5, 0], s: [1.3, 1.2, 1.3] });
  return item('Bottled lightning', [part('bottle', 'body', [0, 0, 0], [bottle]), part('rod and cage', 'trim', [0, .5, .4], [cap, spike, down, knob].concat(cage)), part('lightning', 'detail', [0, 0, .9], bolts.concat([cloud])), part('cork', '', [0, .9, 0], [cork])], { stand: true, yaw: .2, el: 12, lights: [{ p: [0, .6, 0], color: c.detail, power: 1.1 * power }], halo: .9 });
}
