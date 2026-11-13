// Food and drink.
import { THREE, V, M, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, blob, sheet, many, band, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, paper, matte, glass, liquid, bone, ghost } from '../mats.js';

export function bottle(c) {
  const gl = glass(c.body, { tint: .34 }), ck = wood(c.trim, { bump: .3 }), foil = metal(c.detail, { kind: 'cast', rough: .38 });
  const wine = new THREE.Color(c.detail).lerp(new THREE.Color('#2a0508'), .55);
  const body = turned([[0, .05], [.12, .03], [.3, 0], [.335, .03], [.34, .12], [.34, .95], [.31, 1.12], [.2, 1.3], [.125, 1.46], [.11, 1.6], [.11, 1.9], [.135, 1.915], [.135, 1.97], [.1, 1.985]], gl, { sm: 110, seg: 64 });
  const inner = turned([[0, .09], [.28, .06], [.31, .12], [.31, .94], [.285, 1.1], [.21, 1.22], [0, 1.22]], liquid('#' + wine.getHexString(), { lit: .22 }), { sm: 70, seg: 48 });
  const cork = cyl(.085, .092, .2, ck, { p: [0, 2.03, 0] });
  const cap = turned([[.115, 1.62], [.118, 1.9], [.142, 1.91], [.142, 1.975], [.1, 1.99], [.095, 1.95]], foil, { sm: 0, seg: 48 });
  const label = sheet((u, v) => { const a = (u - .5) * 1.9; return V(Math.sin(a) * .346, .34 + v * .46, Math.cos(a) * .346); }, 24, 4, paper('#e4d8b8'));
  return item('Bottle', [part('glass', 'body', [0, 0, 0], [body, inner]), part('cork', 'trim', [0, .5, 0], [cork]), part('foil', 'detail', [0, .25, .5], [cap]), part('label', '', [0, -.1, .6], [label])], { stand: true, yaw: .25, el: 12 });
}

export function bread(c) {
  const crust = matte(c.body, { grain: 'crust', rough: .85, bump: 1.6 }), wd = wood(c.trim, { bump: 1 }), seed = matte(c.detail, { rough: .6 }), n = makeNoise(14, 32), r = rng(9);
  const board = rbox(2.0, .09, 1.15, .03, wd, { p: [0, .045, 0] }), handle = rbox(.5, .09, .3, .03, wd, { p: [1.2, .045, 0] }), hole = cyl(.06, .06, .1, wood('#2a1a10'), { p: [1.3, .046, 0] });
  // Three slashes opened in the oven: the crust lifts in an ear beside each pale split.
  const slash = v => { let k = 0; for (const o of [-.45, 0, .45]) { const d = (v.x - o) * .9 + v.z * .5; k += Math.exp(-d * d / .004) * Math.max(0, v.y) ; } return k; };
  const loaf = blob([.78, .4, .46], v => (1 + (n(v.x * 2 + 1, v.z * 2 + v.y) - .5) * .12 + slash(v) * .1) * (v.y < -.3 ? 1 - (-.3 - v.y) * .9 : 1), crust, { p: [-.15, .34, 0], seg: 120, seg2: 80 });
  const crumb = [-.45, 0, .45].map(o => blob([.045, .06, .3], null, matte('#e8d6a8', { grain: 'crust', bump: .8 }), { r: [0, -.5, 0], p: [-.15 + o * .78 + .01, .73 - Math.abs(o) * .09, 0] }));
  const seeds = many(new THREE.SphereGeometry(.016, 8, 6), seed, Array.from({ length: 150 }, () => { const a = r() * 6.28, d = Math.sqrt(r()) * .85, x = Math.cos(a) * d, z = Math.sin(a) * d * .9, y = Math.sqrt(Math.max(0, 1 - x * x - z * z)); return { p: [-.15 + x * .78, .34 + y * .4 * (1 + .0) + .004, z * .46], r: [r() * 3, r() * 3, 0], s: [1, .5, 1.6] }; }));
  const flour = many(new THREE.SphereGeometry(.012, 6, 4), matte('#f1ead8'), Array.from({ length: 60 }, () => ({ p: [(r() - .5) * 1.8, .092, (r() - .5) * 1.0], s: [1, .3, 1] })));
  return item('Loaf', [part('board', 'trim', [0, -.3, 0], [board, handle, hole, flour]), part('loaf', 'body', [0, .2, 0], [loaf].concat(crumb)), part('seeds', 'detail', [0, .7, 0], [seeds])], { stand: true, yaw: .45, el: 30 });
}

export function tankard(c) {
  const wd = wood(c.body, { bump: 1 }), iron = metal(c.trim, { kind: 'forged', rough: .5 }), foamM = matte(c.detail, { rough: .9, bump: .9, sheen: .6 }), n = makeNoise(27, 32), r = rng(6);
  const cup = turned([[0, 0], [.46, 0], [.49, .03], [.5, .5], [.46, 1.1], [.45, 1.16], [.41, 1.16], [.4, 1.08], [.4, .3], [0, .26]], wd, { sm: 0, seg: 72 });
  const gaps = many(new THREE.BoxGeometry(.012, 1.12, .012), wood('#2a1a10'), Array.from({ length: 14 }, (_, i) => { const a = i / 14 * Math.PI * 2, y = .58; return { p: [Math.cos(a) * .485, y, Math.sin(a) * .485], r: [0, -a, 0] }; }));
  const hoops = [.16, .92].map(y => band(.5 - Math.abs(y - .5) * .06 + (y > .5 ? -.012 : 0), y, .1, .016, iron));
  const handle = rod([[.48, .95, 0], [.82, .98, 0], [.92, .6, 0], [.84, .22, 0], [.5, .2, 0]], .055, wd, { n: 40, sides: 12, s: [1, 1, 1.3] });
  const ale = cyl(.4, .4, .04, liquid('#6a3a12', { lit: .2 }), { p: [0, 1.02, 0] });
  const head = blob([.46, .2, .46], v => 1 + (n(v.x * 4 + 2, v.z * 4 + v.y * 3) - .5) * .35, foamM, { p: [0, 1.14, 0], seg: 72, seg2: 48 });
  const spill = [blob([.16, .3, .1], v => 1 + (n(v.x * 5, v.y * 4) - .5) * .4, foamM, { p: [-.3, .98, .36] }), blob([.1, .18, .07], null, foamM, { p: [-.38, .74, .33] }), blob([.12, .1, .1], null, foamM, { p: [.2, 1.22, .34] })];
  const bubbles = many(new THREE.SphereGeometry(.03, 10, 8), foamM, Array.from({ length: 40 }, () => { const a = r() * 6.28, d = Math.sqrt(r()) * .42; return { p: [Math.cos(a) * d, 1.3 + r() * .05 - d * .2, Math.sin(a) * d], s: .5 + r() }; }));
  return item('Tankard', [part('staves', 'body', [0, -.2, 0], [cup, gaps]), part('hoops', 'trim', [0, 0, .7], hoops), part('handle', 'trim', [.6, 0, 0], [handle]), part('foam', 'detail', [0, .6, 0], [ale, head, bubbles].concat(spill))], { stand: true, yaw: -.5, el: 16 });
}

export function pie(c, t) {
  const crust = matte(c.body, { grain: 'crust', rough: .8, bump: 1.3 }), tin = metal(c.trim, { rough: .4 }), fill = matte(c.detail, { rough: .2, wet: 1 }), n = makeNoise(33, 32), ph = (t || 0) * Math.PI * 2;
  const dish = turned([[0, 0], [.6, 0], [.64, .03], [.84, .3], [.9, .31], [.9, .33], [.82, .33], [.6, .05], [0, .05]], tin, { sm: 0, seg: 72 });
  const top = turned([[0, .56], [.3, .53], [.56, .44], [.74, .36], [.86, .36], [.9, .34], [.88, .31]], crust, { sm: 60, seg: 144 });
  { const q = top.geometry.attributes.position; for (let i = 0; i < q.count; i++) { const x = q.getX(i), y = q.getY(i), z = q.getZ(i), rr = Math.hypot(x, z), a = Math.atan2(z, x), edge = Math.max(0, (rr - .72) / .18), k = 1 + edge * .05 * Math.cos(a * 22); q.setXYZ(i, x * k, y + edge * .035 * Math.cos(a * 22) + (n(x * 3 + 4, z * 3) - .5) * .05 * (1 - edge), z * k); } top.geometry.computeVertexNormals(); }
  const vents = [0, 1, 2, 3, 4].map(i => { const a = i / 5 * Math.PI * 2 + .3; return blob([.15, .03, .045], null, fill, { r: [0, -a, 0], p: [Math.cos(a) * .3, .535, Math.sin(a) * .3] }); });
  const ooze = [0, 1, 2].map(i => { const a = [1.0, 2.9, 4.6][i]; return blob([.09, .04, .07], null, fill, { p: [Math.cos(a) * .8, .375, Math.sin(a) * .8] }); });
  const steam = [0, 1, 2].map(i => { const pts = []; for (let j = 0; j <= 24; j++) { const s = j / 24; pts.push(V(Math.sin(s * 4 + i * 2 + ph) * .08 * s + (i - 1) * .24, .58 + s * .8, Math.cos(s * 3 + i + ph) * .06 * s)); } return rod(pts, s => .012 + .045 * s, ghost('#c8c2b8', .08), { sides: 8 }); });
  return item('Pie', [part('dish', 'trim', [0, -.3, 0], [dish]), part('crust', 'body', [0, .2, 0], [top]), part('filling', 'detail', [0, .6, 0], vents.concat(ooze)), part('steam', '', [0, .9, 0], steam)], { stand: true, yaw: .3, el: 32 });
}

export function roast(c) {
  const meat = matte(c.body, { grain: 'crust', rough: .45, bump: 1.4, wet: .6 }), bn = bone(c.trim), herb = matte(c.detail, { rough: .8, double: true }), n = makeNoise(39, 32), r = rng(2);
  const leg = turned([[0, .8], [.2, .78], [.4, .62], [.5, .36], [.5, .1], [.4, -.2], [.24, -.48], [.13, -.66], [.09, -.74], [0, -.75]], meat, { sm: 80, seg: 72 });
  { const q = leg.geometry.attributes.position; for (let i = 0; i < q.count; i++) { const x = q.getX(i), y = q.getY(i), z = q.getZ(i), k = 1 + (fbm(n, x * 3 + 2, y * 3 + z * 2, 3) - .5) * .22; q.setXYZ(i, x * k, y, z * k * .9); } leg.geometry.computeVertexNormals(); leg.geometry.rotateZ(-Math.PI / 2 + .1); leg.geometry.translate(.2, .46, 0); }
  const shank = turned([[.07, 0], [.06, .3], [.065, .62], [.09, .7], [.11, .78], [.09, .86], [.05, .88], [0, .88]], bn, { sm: 40, r: [0, 0, Math.PI / 2 + .1], p: [-.3, .41, 0] });
  const knob = [ball(.08, bn, { p: [-1.16, .54, .05] }), ball(.075, bn, { p: [-1.18, .47, -.05] })];
  const leaves = many(new THREE.SphereGeometry(.05, 8, 6), herb, Array.from({ length: 30 }, () => { const a = r() * 6.28, d = .1 + r() * .4; return { p: [.45 + Math.cos(a) * d * .7, .95 - d * d * .7 + r() * .02, Math.sin(a) * d * .5], r: [r() * .6, r() * 6, r() * .6], s: [1.4, .18, .5] }; }));
  const twine = [];
  return item('Roast leg', [part('meat', 'body', [.2, 0, 0], [leg].concat(twine)), part('bone', 'trim', [-.5, 0, 0], [shank].concat(knob)), part('herbs', 'detail', [0, .6, 0], [leaves])], { stand: true, yaw: .35, el: 30 });
}

export function stew(c, t) {
  const wd = wood(c.trim, { polish: .3 }), broth = matte(c.body, { rough: .15, wet: 1 }), bit = matte(c.detail, { rough: .5, wet: .5 }), n = makeNoise(44, 16), r = rng(11), ph = (t || 0) * Math.PI * 2;
  const bowl = turned([[0, 0], [.34, 0], [.38, .03], [.4, .08], [.62, .3], [.8, .58], [.82, .7], [.77, .71], [.72, .58], [.5, .3], [.2, .2], [0, .18]], wd, { sm: 120, seg: 72 });
  const soup = cyl(.715, .715, .02, broth, { p: [0, .555, 0], seg: 64 });
  const caps = many(new THREE.SphereGeometry(.1, 16, 10, 0, 6.3, 0, 1.6), bit, Array.from({ length: 7 }, (_, i) => { const a = i * 2.4 + 1, d = .16 + (i % 3) * .17; return { p: [Math.cos(a) * d, .555, Math.sin(a) * d], r: [(r() - .5) * .5, r() * 6, (r() - .5) * .5], s: [1, .6, 1] }; }));
  const chunks = many(new THREE.DodecahedronGeometry(.07, 0), matte('#8a4a22', { rough: .5, wet: .6 }), Array.from({ length: 9 }, (_, i) => { const a = i * 1.9, d = .1 + (i % 4) * .15; return { p: [Math.cos(a) * d, .56, Math.sin(a) * d], r: [r() * 3, r() * 3, r() * 3], s: .7 + r() * .6 }; }));
  const greens = many(new THREE.SphereGeometry(.03, 6, 4), matte('#5f8a36'), Array.from({ length: 24 }, () => { const a = r() * 6.28, d = Math.sqrt(r()) * .62; return { p: [Math.cos(a) * d, .572, Math.sin(a) * d], r: [0, r() * 6, 0], s: [1.5, .2, .6] }; }));
  const spoon = [turned([[.022, 0], [.028, .5], [.02, 1.0], [.03, 1.1], [0, 1.12]], wd, { sm: 20, seg: 12, r: [0, 0, -.95], p: [.22, .58, .1] }), blob([.12, .03, .09], null, wd, { p: [.2, .59, .1] })];
  const steam = [0, 1, 2].map(i => { const pts = []; for (let j = 0; j <= 24; j++) { const s = j / 24; pts.push(V(Math.sin(s * 4 + i * 2 + ph) * .09 * s + (i - 1) * .25 - .1, .6 + s * .9, Math.cos(s * 3 + i + ph) * .07 * s)); } return rod(pts, s => .012 + .05 * s, ghost('#c8c2b8', .08), { sides: 8 }); });
  return item('Hot meal', [part('bowl', 'trim', [0, -.3, 0], [bowl]), part('broth', 'body', [0, .1, 0], [soup]), part('what went in', 'detail', [0, .5, 0], [caps, chunks, greens]), part('spoon', 'trim', [.6, .4, 0], spoon), part('steam', '', [0, .9, 0], steam)], { stand: true, yaw: .2, el: 34 });
}
