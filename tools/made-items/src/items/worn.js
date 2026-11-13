// Things worn or carried on the body.
import { THREE, V, M, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, slab, shape, circle, blob, sheet, fan, rim, chain, many, aim, cutStone, strands, wrap, band, tube, path, loft, ring as ringOf, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, gem, bone, matte, dark } from '../mats.js';
import { tex } from '../util.js';

export function shield(c) {
  const face = leather(c.body, { bump: .6 }), steel = metal(c.trim), gilt = metal(c.detail, { kind: 'cast' });
  const out = [[0, 1.05], [.5, 1.0], [.82, .78], [.86, .3], [.7, -.35], [.38, -.95], [0, -1.4], [-.38, -.95], [-.7, -.35], [-.86, .3], [-.82, .78], [-.5, 1.0]];
  const zf = (x, y) => .3 * (1 - Math.pow(x / .95, 2)) + .03 * (1 - Math.pow((y + .2) / 1.3, 2));
  const body = fan(out, zf, face, { cy: -.1, nu: 120, nv: 30, uvx: 3, uvy: 2 }), edge = rim(out, zf, .034, steel, { lift: .004, n: 160 });
  const boss = turned([[0, .16], [.08, .15], [.15, .1], [.19, .03], [.26, .02], [.27, 0]], steel, { sm: 40, r: [Math.PI / 2, 0, 0], p: [0, .05, zf(0, .05) - .005] });
  const rv = []; for (let i = 0; i < 22; i++) { const a = i / 22 * Math.PI * 2, x = Math.sin(a) * .7 * (1 - .32 * Math.max(0, -Math.cos(a))), y = -.15 + Math.cos(a) * (Math.cos(a) > 0 ? 1.02 : 1.08); rv.push({ p: [x, y, zf(x, y) + .004], s: [1, 1, .6] }); }
  const rivets = many(new THREE.SphereGeometry(.022, 10, 8), steel, rv);
  // The charge: a gilt chevron over the boss and a bar under it, following the curve of the face.
  const strip = (pts, w) => sheet((u, v) => { const n = pts.length - 1, i = Math.min(n - 1, Math.floor(u * n)), f = u * n - i, x = pts[i][0] + (pts[i + 1][0] - pts[i][0]) * f, y = pts[i][1] + (pts[i + 1][1] - pts[i][1]) * f + (v - .5) * w; return V(x, y, zf(x, y) + .012); }, 40, 2, gilt);
  const charge = [strip([[-.62, .32], [0, .72], [.62, .32]], .13), strip([[-.5, -.42], [0, -.2], [.5, -.42]], .11), strip([[-.3, -.82], [0, -.68], [.3, -.82]], .09)];
  return item('Kite shield', [part('face', 'body', [0, 0, -.2], [body]), part('charge', 'detail', [0, 0, .5], charge), part('rim', 'trim', [0, 0, .2], [edge, rivets]), part('boss', 'trim', [0, 0, .8], [boss])], { yaw: .3, fill: .92 });
}

export function gauntlet(c) {
  const steel = metal(c.body, { rough: .34 }), br = metal(c.trim, { kind: 'cast' }), lin = leather(c.detail);
  const seg = (r0, r1, h, sz) => { const g = lathe(smooth([[r0 * .86, 0], [r0, h * .08], [(r0 + r1) / 2 * 1.06, h * .5], [r1, h * .94], [r1 * .9, h]], 16), 20); g.scale(1, 1, sz || .8); return g; };
  const fingers = [];
  [[-.27, .52, .11, .1, .92], [-.09, .6, .115, 0, 1.06], [.1, .58, .11, -.07, 1.0], [.28, .5, .1, -.16, .84]].forEach(([x, y, r, lean, len]) => {
    let p = V(x, y, .02), ang = lean;
    [.36, .3, .26].forEach((h, i) => { const hh = h * len, g = seg(r * (1 - i * .1), r * (1 - (i + 1) * .1), hh, .8); g.rotateX(-.08 * i); g.rotateZ(ang); g.translate(p.x, p.y, p.z - i * .012); fingers.push(new THREE.Mesh(g, steel)); p = p.clone().add(V(-Math.sin(ang) * hh * .93, Math.cos(ang) * hh * .93, 0)); ang += lean * .25; });
    fingers.push(ball(r * .72, steel, { s: [1, 1.1, .8], p: [p.x, p.y, p.z - .03] }));
  });
  const thumb = []; { let p = V(-.42, -.12, .0), ang = .95; [.34, .3].forEach((h, i) => { const g = seg(.13 * (1 - i * .12), .12 * (1 - i * .15), h, .82); g.rotateZ(ang); g.translate(p.x, p.y, p.z); thumb.push(new THREE.Mesh(g, steel)); p = p.clone().add(V(-Math.sin(ang) * h * .93, Math.cos(ang) * h * .93, 0)); ang -= .25; }); thumb.push(ball(.085, steel, { s: [1, 1.1, .8], p: [p.x, p.y, p.z] })); }
  const hand = blob([.47, .5, .2], v => 1 + .06 * Math.cos(v.x * 9) * Math.max(0, v.y), steel, { p: [0, .12, 0] });
  const knuck = [-.27, -.09, .1, .28].map((x, i) => ball(.105, steel, { s: [1, .9, .75], p: [x, .5 + [0, .07, .05, -.02][i], .12] }));
  const cuff = turned([[.3, -.3], [.31, -.5], [.38, -.9], [.46, -1.25], [.48, -1.3]], steel, { sm: 30, seg: 48, s: [1, 1, .62] });
  const lames = [-.38, -.2].map(y => turned([[.33, y - .12], [.345, y], [.31, y + .1]], steel, { sm: 12, seg: 48, s: [1, 1, .64] }));
  const bands = [band(.47, -1.26, .07, .016, br, { s: [1, 1, .62] }), band(.325, -.52, .05, .012, br, { s: [1, 1, .62] })];
  const lining = turned([[.44, -1.3], [.4, -1.2], [0, -1.18]], lin, { sm: 0, seg: 40, s: [1, 1, .6] });
  const rv = many(new THREE.SphereGeometry(.02, 8, 6), br, [-.3, -.1, .1, .3].map(x => ({ p: [x, -1.1, Math.sqrt(Math.max(0, 1 - Math.pow(x / .44, 2))) * .27 + .005] })));
  return item('Gauntlet', [part('fingers', 'body', [0, .4, 0], fingers.concat(knuck)), part('thumb', 'body', [-.4, 0, 0], thumb), part('hand plate', 'body', [0, 0, .3], [hand].concat(lames)), part('cuff', 'body', [0, -.35, 0], [cuff]), part('bands', 'trim', [.5, -.3, 0], bands.concat([rv])), part('lining', 'detail', [0, -.8, 0], [lining])], { yaw: -.35, fill: .9 });
}

export function cloak(c) {
  const wool = cloth(c.body, { bump: .9, sheen: .5 }), hemM = cloth(c.trim, { sheen: 1 }), gold = metal(c.trim, { kind: 'cast' }), st = gem(c.detail, { round: true, edges: 0 }), n = makeNoise(12, 32);
  // Hanging as if from shoulders: open down the front, falling in deep folds that widen to the hem.
  const drape = (u, v, out) => { const a = .42 + u * (Math.PI * 2 - .84), sh = Math.min(1, v / .16), r0 = .16 + .3 * Math.sin(sh * Math.PI / 2) + .34 * v, fold = Math.sin(a * 7 + n(u * 5, 3) * 4) * (.02 + .1 * v) + (n(u * 11, v * 3) - .5) * .06 * v, r = r0 + fold + out, y = 1.72 - .2 * (1 - Math.cos(sh * Math.PI / 2)) - Math.max(0, v - .0) * 1.52 + .03 * Math.sin(a * 7) * v; return V(Math.sin(a) * r * 1.12, y, -Math.cos(a) * r * .8); };
  const body = sheet((u, v) => drape(u, v, 0), 200, 50, wool, { uvx: 9, uvy: 6 });
  const hem = sheet((u, v) => drape(u, .95 + v * .055, .006), 200, 3, hemM), edges = [0, 1].map(k => sheet((u, v) => drape(k ? 1 - u * .012 : u * .012, v, .007), 2, 50, hemM));
  const hood = blob([.3, .2, .26], v => 1 + (n(v.x * 2 + 4, v.y * 2 + v.z) - .5) * .5 + .2 * Math.max(0, -v.y), wool, { p: [0, 1.6, -.22] }), collar = torus(.19, .06, wool, { r: [Math.PI / 2, 0, 0], s: [1.1, 1, .8], p: [0, 1.7, 0] });
  const cl = [-1, 1].map(k => turned([[0, .03], [.05, .026], [.07, .01], [.075, 0]], gold, { sm: 16, r: [Math.PI / 2, 0, 0], p: [k * .1, 1.62, .2] }));
  const ch = chain([[-.09, 1.6, .22], [0, 1.55, .25], [.09, 1.6, .22]], .014, gold), stone = ball(.04, st, { s: [1, 1, .6], p: [-.1, 1.62, .235] });
  return item('Cloak', [part('cloth', 'body', [0, -.2, 0], [body]), part('hem', 'trim', [0, -.5, .2], [hem].concat(edges)), part('hood', 'body', [0, .4, -.3], [hood, collar]), part('clasp', 'detail', [0, .1, .7], cl.concat([ch, stone]))], { stand: true, yaw: .25, el: 12 });
}

export function breastplate(c) {
  const steel = metal(c.body, { rough: .44, double: true }), gold = metal(c.trim, { kind: 'cast' }), st = gem(c.detail, { inner: .3, edges: .1 }), hide = leather('#3a2a22', { double: true });
  const out = [[0, .78], [.16, .82], [.3, .98], [.44, 1.08], [.56, 1.02], [.6, .8], [.66, .56], [.84, .36], [.9, .1], [.8, -.3], [.66, -.62], [.64, -.86], [.34, -.94], [0, -.98], [-.34, -.94], [-.64, -.86], [-.66, -.62], [-.8, -.3], [-.9, .1], [-.84, .36], [-.66, .56], [-.6, .8], [-.56, 1.02], [-.44, 1.08], [-.3, .98], [-.16, .82]];
  const zf = (x, y) => .56 * Math.pow(Math.max(0, 1 - Math.pow(Math.abs(x) / 1.0, 1.7)), .8) * (1 - .3 * Math.pow((y - .15) / 1.2, 2)) + .045 * Math.exp(-Math.pow(x / .04, 2)) * (y < .62 ? 1 : 0);
  const plate = fan(out, zf, steel, { nu: 200, nv: 40, uvx: 2, uvy: 2, cy: .05, tension: .35 }), edge = rim(out, zf, .026, gold, { n: 260, lift: .002, tension: .35 });
  const fauld = [0, 1].map(i => sheet((u, v) => { const x = (u - .5) * (1.3 + i * .12), y = -.9 - i * .17 - v * .22 + .05 * Math.cos(x * 2.4); return V(x, y, zf(x * .92, -.8) * (1 + i * .04) + .02 - i * .012 - v * .03); }, 40, 4, steel));
  const waist = sheet((u, v) => { const x = (u - .5) * 1.26, y = -.72 - v * .09 + .05 * Math.cos(x * 2.4); return V(x, y, zf(x, y) + .012); }, 40, 2, gold);
  const straps = [-1, 1].map(k => sheet((u, v) => { const x = k * (.37 + u * .14), y = 1.0 + v * .42; return V(x, y, zf(x, 1.0) - v * .3 + .01); }, 3, 10, hide));
  const bez = turned([[0, .03], [.1, .03], [.125, .015], [.13, 0]], gold, { sm: 16, r: [Math.PI / 2, 0, 0], p: [0, .34, zf(0, .34) - .004] });
  const stone = M(cutStone(.09, .035, .03), st, { p: [0, .34, zf(0, .34) + .032] });
  return item('Breastplate', [part('plate', 'body', [0, 0, 0], [plate].concat(fauld)), part('collar and waist', 'trim', [0, 0, .4], [edge, waist]), part('straps', 'trim', [0, .5, 0], straps), part('heart stone', 'detail', [0, 0, .9], [bez, stone])], { stand: true, yaw: .16, el: 10, fill: .82 });
}

export function ring(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .2 }), bz = metal(c.trim, { kind: 'cast', rough: .2 }), st = gem(c.detail, { inner: .3, edges: .1 });
  const bandG = torus(.5, .075, gold, { s: [1, 1, 1.5], seg: 80 });
  const sh = [-1, 1].map(k => blob([.2, .16, .14], null, gold, { p: [k * .18, .5, 0] }));
  const bezel = turned([[0, .56], [.25, .56], [.3, .6], [.31, .68], [.27, .71], [.22, .7], [.21, .66], [0, .66]], bz, { sm: 0, seg: 8, s: [1, 1, .78] });
  const stone = M(cutStone(.2, .06, .08), st, { r: [-Math.PI / 2, 0, 0], s: [1, 1, .78], p: [0, .7, 0] });
  return item('Signet ring', [part('band', 'body', [0, -.2, 0], [bandG].concat(sh)), part('bezel', 'trim', [0, .25, 0], [bezel]), part('stone', 'detail', [0, .7, 0], [stone])], { stand: true, yaw: .65, el: 30, fill: .8 });
}

export function crown(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .2 }), st = gem(c.detail, { round: true, edges: 0 }), velvet = cloth(c.detail, { sheen: 1, bump: .2 }), R = .5;
  const bandM = turned([[R, 0], [R + .03, .01], [R + .035, .06], [R + .015, .08], [R + .015, .2], [R + .035, .22], [R + .03, .27], [R, .28], [R - .015, .14]], gold, { sm: 0, seg: 72 });
  const pts = [], pearls = [], stones = [];
  for (let i = 0; i < 8; i++) { const a = i / 8 * Math.PI * 2, tall = i % 2 === 0, h = tall ? .52 : .3;
    pts.push(slab([[-.13, 0], [.13, 0], [.09, h * .3], [.12, h * .55], [.05, h * .72], [0, h], [-.05, h * .72], [-.12, h * .55], [-.09, h * .3]], .03, .012, gold, { curved: false, p: [0, .26, R + .005], r: [0, 0, 0] }));
    pts[i].geometry.rotateY(a); pearls.push({ p: [Math.sin(a) * (R + .005), .26 + h + .035, Math.cos(a) * (R + .005)] });
    stones.push(ball(.05, st, { s: [1, 1.2, .6], p: [0, .14, R + .03] })); stones[i].geometry.rotateY(a + Math.PI / 8); }
  const balls = many(new THREE.SphereGeometry(.04, 14, 10), gold, pearls);
  const cap = blob([R - .03, .3, R - .03], v => v.y < 0 ? .2 : 1 + .05 * Math.cos(Math.atan2(v.z, v.x) * 8) * (1 - v.y), cloth('#' + new THREE.Color(c.detail).multiplyScalar(.6).getHexString(), { sheen: 1, bump: .2 }), { p: [0, .2, 0] });
  return item('Crown', [part('band', 'trim', [0, -.2, 0], [bandM]), part('points', 'body', [0, .3, 0], pts.concat([balls])), part('stones', 'detail', [0, 0, .7], stones), part('cap', 'detail', [0, .7, 0], [cap])], { stand: true, yaw: .3, el: 22 });
}

export function mask(c) {
  const bn = bone(c.body, { gloss: .3, double: true }), gold = metal(c.trim, { kind: 'cast' }), silk = cloth(c.detail, { sheen: 1, bump: .2 }), ss = x => { x = Math.max(0, Math.min(1, x)); return x * x * (3 - 2 * x); }, G = (d, w) => Math.exp(-(d * d) / (w * w));
  // A carved face: brow, nose, cheekbones and lips stand up from the dome; the eyes and mouth are cut right through.
  const zf = (x, y) => { const ax = Math.abs(x), dome = .36 * Math.sqrt(Math.max(0, 1 - Math.pow(x / .84, 2))) * (1 - .22 * Math.pow(y / 1.1, 2)), nose = .19 * ss((.36 - y) / .52) * (1 - ss((-.2 - y) / .09)) * G(x, .055 + .06 * ss((.3 - y) / .5)), brow = .05 * G(y - .36, .07) * (1 - .6 * G(x, .1)) * (1 - ss((ax - .5) / .2)), eye = -.06 * G(ax - .3, .17) * G(y - .2, .1), cheek = .055 * G(ax - .4, .17) * G(y + .1, .17), lips = .04 * G(x, .17) * (G(y + .5, .04) + G(y + .66, .045)), chin = .045 * G(x, .17) * G(y + .88, .1); return dome + nose + brow + eye + cheek + lips + chin; };
  const out = [[0, .95], [.4, .9], [.68, .6], [.74, .15], [.62, -.35], [.4, -.8], [.16, -1.02], [0, -1.06], [-.16, -1.02], [-.4, -.8], [-.62, -.35], [-.74, .15], [-.68, .6], [-.4, .9]];
  const ey = (x, y, k) => { const ca = Math.cos(k * .14), sa = Math.sin(k * .14), dx = x - k * .3, dy = y - .2, u = dx * ca + dy * sa, v = -dx * sa + dy * ca; return Math.hypot(u / .17, v / .072); };
  const holes = tex(512, 512, (u, v) => { const x = (u - .5) * 1.6, y = (v - .5) * 2.2, e = Math.min(ey(x, y, 1), ey(x, y, -1)), m = Math.hypot(x / .19, (y + .585 + x * x * .5) / .022); return Math.min(e, m) < 1 ? 0 : 1; });
  bn.alphaMap = holes; bn.alphaTest = .5;
  const face = fan(out, zf, bn, { nu: 180, nv: 60 }), fp = face.geometry.attributes.position, fu = face.geometry.attributes.uv; for (let i = 0; i < fp.count; i++) fu.setXY(i, fp.getX(i) / 1.6 + .5, fp.getY(i) / 2.2 + .5);
  const edge = rim(out, zf, .022, bn, { n: 160 });
  const line = (pts, r, closed) => rod(pts.map(q => [q[0], q[1], zf(q[0], q[1]) + .004]), r, gold, { n: closed ? 0 : 40, sides: 8, closed: closed });
  const loop = k => { const ca = Math.cos(k * .14), sa = Math.sin(k * .14), p = []; for (let i = 0; i < 48; i++) { const a = i / 48 * Math.PI * 2, u = Math.cos(a) * .182, v = Math.sin(a) * .084; p.push([k * .3 + u * ca - v * sa, .2 + u * sa + v * ca]); } return line(p, .013, true); };
  const inlay = [loop(1), loop(-1), line([[-.62, .4], [-.34, .5], [-.1, .42]], .014), line([[.62, .4], [.34, .5], [.1, .42]], .014), line([[-.56, -.2], [-.42, -.5], [-.2, -.78]], .011), line([[.56, -.2], [.42, -.5], [.2, -.78]], .011), line([[0, .92], [0, .7], [0, .56]], .013), line([[-.2, .84], [-.32, .7], [-.52, .62]], .009), line([[.2, .84], [.32, .7], [.52, .62]], .009)];
  const ribbons = [-1, 1].map(k => sheet((u, v) => V(k * (.72 + v * .9), .2 + (u - .5) * .09 - v * v * .5 + Math.sin(v * 9) * .04, .06 + Math.sin(v * 7 + k) * .05 * v + .01), 3, 40, silk));
  return item('Mask', [part('face', 'body', [0, 0, 0], [face, edge]), part('inlay', 'trim', [0, 0, .5], inlay), part('ribbons', 'detail', [0, -.4, -.1], ribbons)], { yaw: .14, fill: .9, prop: .12 });
}

export function pouch(c) {
  const hide = leather(c.body), flapM = leather(c.detail, { double: true }), br = metal(c.trim, { kind: 'cast' }), n = makeNoise(21, 16), cord = matte('#c9b08a', { rough: .8 });
  const se = (v, e) => Math.sign(v) * Math.pow(Math.abs(v), e);
  const bag = blob([.6, .52, .26], v => { const k = 1 / Math.pow(Math.pow(Math.abs(v.x), 4) + Math.pow(Math.abs(v.y), 4) + Math.pow(Math.abs(v.z), 2.4), 1 / 3.4); return Math.min(1.45, k) * (1 + (n(v.x * 3 + 5, v.y * 3 + v.z * 2) - .5) * .07) * (1 + .1 * Math.max(0, -v.y)); }, hide, { p: [0, .58, 0], seg: 80, seg2: 56 });
  const fz = (x, y) => .3 * Math.pow(Math.max(0, 1 - Math.pow(Math.abs(x) / .7, 3)), .5) + .012;
  const flap = sheet((u, v) => { const x = (u - .5) * 1.16, drop = .62 * (1 - .32 * Math.pow(Math.abs(x) / .58, 2.2)), t = v, y = t < .3 ? 1.12 + Math.sin(t / .3 * Math.PI / 2) * .0 : 1.12 - (t - .3) / .7 * drop, z = t < .3 ? -.24 + (t / .3) * .24 + fz(x, y) * Math.sin(t / .3 * Math.PI / 2) : fz(x, y) + .02 * Math.sin((t - .3) / .7 * Math.PI); return V(x, y + (t < .3 ? .03 * Math.sin(t / .3 * Math.PI) : 0), z); }, 40, 30, flapM);
  const stitch = []; for (let i = 0; i <= 40; i++) { const u = i / 40, x = (u - .5) * 1.08, drop = .62 * (1 - .32 * Math.pow(Math.abs(x) / .58, 2.2)); stitch.push({ p: [x, 1.12 - drop + .045, fz(x, 0) + .03], r: [0, 0, Math.atan2(-(.62 * .32 * 2.2 * Math.pow(Math.abs(x) / .58, 1.2) / .58) * Math.sign(x), 1)], s: [1, 1, 1] }); }
  const stitches = many(new THREE.CapsuleGeometry(.007, .028, 4, 6).rotateZ(Math.PI / 2), cord, stitch);
  const stud = turned([[0, .05], [.035, .045], [.055, .02], [.06, 0]], br, { sm: 16, r: [Math.PI / 2, 0, 0], p: [0, .62, fz(0, 0) + .03] });
  const loops = [-1, 1].map(k => rbox(.16, .46, .05, .02, leather(c.detail), { p: [k * .3, 1.16, -.26] }));
  return item('Belt pouch', [part('pouch', 'body', [0, -.2, 0], [bag]), part('flap', 'body', [0, .15, .5], [flap]), part('stud', 'trim', [0, 0, 1], [stud]), part('loops', 'detail', [0, .35, -.4], loops), part('stitching', 'trim', [0, -.1, .8], [stitches])], { stand: true, yaw: .5, el: 16 });
}

export function helm(c) {
  const steel = metal(c.body, { rough: .3, double: true }), br = metal(c.trim, { kind: 'cast' }), hair = cloth(c.detail, { sheen: 1 });
  const dome = turned([[0, 1.3], [.12, 1.28], [.3, 1.16], [.44, .96], [.5, .72], [.51, .5]], steel, { sm: 60, seg: 72 });
  const skirt = M(new THREE.LatheGeometry(smooth([[.51, .52], [.52, .3], [.53, .05], [.57, -.04]], 20).map(p => new THREE.Vector2(p[0], p[1])), 60, .62, Math.PI * 2 - 1.24), steel);
  const brow = band(.512, .51, .09, .022, br);
  const crestPts = []; for (let i = 0; i <= 20; i++) { const s = i / 20, a = -1.25 + s * 2.5; const y = .5 + .82 * Math.cos(a * .62) * (1 - Math.pow(Math.abs(a) / 1.6, 2.2)), z = Math.sin(a) * .52 * (1 - .22 * Math.cos(a)); crestPts.push(V(0, Math.max(.52, y) + .02, z)); }
  const comb = M(tube(crestPts, .028, 8), br);
  const nasal = slab([[-.05, .56], [.05, .56], [.06, .1], [.04, -.02], [-.04, -.02], [-.06, .1]], .03, .012, br, { p: [0, 0, .515] });
  // A horsehair crest: a dense brush standing off the ridge, leaning back, longer and looser toward the rear.
  const plume = strands(360, (r, i) => { const k = 2 + r() * 15, q = crestPts[Math.floor(k)].clone().lerp(crestPts[Math.min(20, Math.floor(k) + 1)], k % 1); q.userData = k / 20; return q.add(V((r() - .5) * .035, .015, 0)); }, (r, i, a) => { const back = 1 - (a.z + .5), up = V(0, a.y - .35, a.z * .9).normalize(), len = .2 + .1 * back + r() * .07; return a.clone().addScaledVector(up, len).add(V((r() - .5) * .1, -.02 * back, -.05 - .13 * back * (.6 + r() * .6))); }, .0062, hair, 5);
  const rv = []; for (let i = 0; i < 18; i++) { const a = i / 18 * Math.PI * 2; rv.push({ p: [Math.sin(a) * .535, .51, Math.cos(a) * .535] }); }
  const rivets = many(new THREE.SphereGeometry(.02, 8, 6), br, rv);
  const inside = turned([[0, 1.22], [.3, 1.08], [.46, .7], [.47, .1]], dark(), { sm: 30, seg: 40 });
  return item('Helm', [part('bowl', 'body', [0, 0, 0], [dome, skirt, inside]), part('brow and ridge', 'trim', [0, .1, .6], [brow, comb, nasal, rivets]), part('crest', 'detail', [0, .6, -.2], plume)], { stand: true, yaw: .55, el: 12 });
}
