// Things adventurers carry and find: a wizard's hat, a coil of rope, cheese, a fish, honey, lockpicks.
import { THREE, V, M, X, Y, Z, part, item, turned, rod, ball, torus, cyl, rbox, slab, blob, sheet, many, aim, blade, band, drawn, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, paper, matte, glass, liquid, shell, wax, dark } from '../mats.js';
import { tex } from '../util.js';

const TAU = Math.PI * 2;
const star = (R, r) => Array.from({ length: 10 }, (_, i) => { const a = i / 10 * TAU + Math.PI / 2, k = i % 2 ? r : R; return [Math.cos(a) * k, Math.sin(a) * k]; });

export function hat(c) {
  const felt = cloth(c.body, { bump: .9, sheen: .9 }), strap = leather(c.trim, { bump: .8 }), gold = metal(c.detail, { kind: 'cast', rough: .3 }), n = makeNoise(12, 16);
  // The crown is a tall cone that has slumped: it leans, folds once and ends in a tip that hangs.
  const spine = new THREE.CatmullRomCurve3([[0, .1, 0], [0, .5, 0], [.04, .86, 0], [.16, 1.2, .02], [.36, 1.42, .05], [.56, 1.4, .1], [.64, 1.26, .13]].map(p => V(p[0], p[1], p[2])));
  const rad = s => .47 * Math.pow(1 - s, 1.25) + .012, N = 90, A = 56, rings = [];
  let nn = V(1, 0, 0);
  for (let i = 0; i <= N; i++) {
    const s = i / N, p = spine.getPointAt(s), t = spine.getTangentAt(s); nn = nn.clone().addScaledVector(t, -nn.dot(t)).normalize(); const bn = new THREE.Vector3().crossVectors(t, nn), pts = [];
    for (let j = 0; j < A; j++) { const a = j / A * TAU, fold = 1 + .05 * Math.sin(s * 30 + Math.cos(a) * 2) * Math.min(1, s * 4) * (1 - s) + (n(a * 2.5 + 3, s * 7) - .5) * .09 * (1 - s); pts.push([Math.cos(a) * rad(s) * fold, Math.sin(a) * rad(s) * fold]); }
    rings.push(ring(p, nn, bn, pts));
  }
  const crownG = loft(rings); { const uv = crownG.attributes.uv; for (let i = 0; i < uv.count; i++) uv.setXY(i, uv.getX(i) * 7, uv.getY(i) * 6); }
  const crown = M(crownG, felt); crown.material.side = THREE.DoubleSide;
  // A wide brim that has lost its shape: it dips at the front and curls at one side.
  const brim = sheet((u, v) => { const a = u * TAU, r = .44 + v * .62, wave = .09 * Math.sin(a * 2 + .6) + .05 * Math.sin(a * 5 + 2); return V(Math.cos(a) * r, .1 + v * v * (wave - .06) + (n(a * 3 + 9, v * 3) - .5) * .02 * v, Math.sin(a) * r); }, 120, 14, felt, { uvx: 8, uvy: 2 });
  const belt = band(.455, .2, .13, .012, strap), keeper = band(.468, .2, .05, .008, strap, { r: [0, 0, 0] });
  const buckle = [rbox(.2, .19, .02, .008, gold, { p: [0, .2, .475] }), rbox(.12, .11, .024, .004, strap, { p: [0, .2, .476] }), rbox(.012, .13, .03, .004, gold, { p: [0, .2, .482] })];
  const stars = [[.9, .34, .13], [-.5, .5, .08], [.2, .62, .07], [1.5, .56, .06], [-1.3, .36, .07], [.55, .8, .05], [-.2, .9, .045]].map(([a, y, s]) => { const p = spine.getPointAt(Math.min(.5, (y - .1) / 1.9)), r = rad((y - .1) / 1.9 * 1.05) + .012, m = slab(star(s, s * .45), .012, .004, gold); m.geometry.rotateX(-.3); m.geometry.rotateY(Math.PI / 2 - a); m.geometry.translate(p.x + Math.cos(a) * r, y, Math.sin(a) * r); return m; });
  return item('Wizard\'s hat', [part('crown', 'body', [0, .4, 0], [crown]), part('brim', 'body', [0, -.2, 0], [brim]), part('band', 'trim', [0, 0, .6], [belt, keeper, buckle[1]]), part('buckle and stars', 'detail', [0, .2, 1], [buckle[0], buckle[2]].concat(stars))], { stand: true, yaw: -.35, el: 16 });
}

export function rope(c) {
  const hemp = matte(c.body, { rough: .95, bump: 1.2, sheen: .3 }), strap = leather(c.trim, { bump: .9 }), thread = matte(c.detail, { rough: .8, bump: .6 }), br = metal('#b98a3a', { kind: 'cast', rough: .35 }), r = rng(8), n = makeNoise(3, 16);
  // Three strands laid round each other, wound into a coil that sags a little under its own weight.
  const R0 = .62, loops = 6, per = 90, T = .052, centre = [], normal = [];
  for (let i = 0; i <= loops * per; i++) { const s = i / per, a = s * TAU, layer = Math.floor(s / 3), k = s % 3, rr = R0 + (k < 1.5 ? k : 3 - k) * .075 + (n(a * .8 + 2, s) - .5) * .04; centre.push(V(Math.cos(a) * rr, T * 1.1 + layer * T * 2.05 + Math.sin(k / 3 * Math.PI) * .012 + (n(a + 7, s * .5) - .5) * .02, Math.sin(a) * rr * .96)); normal.push(V(Math.cos(a), 0, Math.sin(a))); }
  // The loose end leaves the top of the coil, crosses it and lies on the table.
  const tail = path([centre[centre.length - 1], V(.5, .3, .25), V(.1, .34, .62), V(-.2, .2, .86), V(-.5, .06, 1.0), V(-.86, T, 1.06), V(-1.12, T, .94)], 70);
  const lay = (pts, norm) => { const out = [[], [], []]; for (let i = 0; i < pts.length; i++) { const a = pts[Math.max(0, i - 1)], b = pts[Math.min(pts.length - 1, i + 1)], t = b.clone().sub(a).normalize(), nn = (norm ? norm[i] : V(0, 1, 0)).clone().addScaledVector(t, -(norm ? norm[i] : V(0, 1, 0)).dot(t)).normalize(), bn = new THREE.Vector3().crossVectors(t, nn); for (let k = 0; k < 3; k++) { const ph = i * .55 + k * TAU / 3; out[k].push(pts[i].clone().addScaledVector(nn, Math.cos(ph) * T * .5).addScaledVector(bn, Math.sin(ph) * T * .5)); } } return out.map(p => M(tube(p, T * .56, 7), hemp)); };
  const coil = lay(centre, normal).concat(lay(tail));
  // Whipped ends keep it from fraying; a strap with a buckle keeps the coil together.
  const e = tail[tail.length - 1], e0 = tail[tail.length - 5], whip = [rod([e0, e], T * 1.12, thread, { sides: 12 }), ball(T * 1.05, thread, { p: [e.x, e.y, e.z] })];
  const fray = Array.from({ length: 9 }, () => rod([e, e.clone().add(V(-.1 - r() * .06, (r() - .3) * .04, (r() - .5) * .12))], .006, hemp, { sides: 4 }));
  const tie = a => { const cx = Math.cos(a) * (R0 + .075), cz = Math.sin(a) * (R0 + .075) * .96, pts = []; for (let i = 0; i <= 28; i++) { const q = i / 28 * TAU, w = .16, h = .15; pts.push(V(cx + Math.cos(a) * Math.cos(q) * w, .13 + Math.sin(q) * h, cz + Math.sin(a) * Math.cos(q) * w)); } const m = M(tube(pts, .024, 8, true), strap); m.geometry.scale(1, 1, 1); return m; };
  const flat = a => { const m = tie(a); const p = m.geometry.attributes.position; return m; };
  const a1 = 2.5, a2 = 5.4, ties = [flat(a1), flat(a2)], bk = [a1].map(a => rbox(.1, .1, .03, .01, br, { r: [0, -a + Math.PI / 2, 0], p: [Math.cos(a) * (R0 + .25), .14, Math.sin(a) * (R0 + .25) * .96] }));
  return item('Coil of rope', [part('rope', 'body', [0, 0, 0], coil.concat(fray)), part('straps', 'trim', [0, .5, 0], ties.concat(bk)), part('whipped end', 'detail', [-.5, 0, .6], whip)], { stand: true, yaw: .5, el: 34 });
}

export function cheese(c) {
  const rind = wax(c.body, { lit: .02 }), paste = matte(c.trim, { rough: .62, bump: .5, wet: .12 }), hole = matte('#' + new THREE.Color(c.trim).multiplyScalar(.45).getHexString(), { rough: .8 }), wd = wood(c.detail, { bump: .8 }), steel = metal('#aab3bd', { kind: 'steel' }), r = rng(21), H2 = Math.PI / 2;
  const R = .82, H = .44, cut = 1.05, prof = [[0, 0], [R - .06, 0], [R, .06], [R + .012, H / 2], [R, H - .06], [R - .06, H], [0, H]];
  // A turned solid starts at +Z and turns toward +X, so a cut face at angle a lies along (sin a, cos a).
  const turn = arc => { const m = M(new THREE.LatheGeometry(prof.map(p => new THREE.Vector2(p[0], p[1])).reverse(), 80, 0, arc), rind); m.material.side = THREE.DoubleSide; return m; };
  const face = a => { const m = slab(prof, .012, 0, paste); m.geometry.rotateY(a - H2); return m; };
  const pocks = (a, count, out) => { for (let i = 0; i < count; i++) { const rr = .12 + r() * .6, y = .08 + r() * (H - .16), s = .02 + r() * .04; out.push({ p: V(Math.sin(a) * rr, y, Math.cos(a) * rr), ry: a + H2, s: [s, s, .014] }); } return out; };
  // The wheel with a wedge cut from it: the rind outside, the paste on the two cut faces.
  const wheel = turn(TAU - cut), faces = [face(0), face(TAU - cut)], holes = pocks(TAU - cut, 9, pocks(0, 9, []));
  // The wedge lies in front of the wheel with the knife that cut it.
  const wa = .62, th = 2.4, at = V(.45, 0, 1.5), wedge = turn(wa), wf = [face(0), face(wa)], wh = pocks(wa, 7, pocks(0, 6, []));
  for (const m of [wedge].concat(wf)) { m.geometry.rotateY(th); m.geometry.translate(at.x, at.y, at.z); }
  for (const h of wh) { h.p.applyAxisAngle(Y, th).add(at); h.ry += th; }
  const dots = many(new THREE.SphereGeometry(1, 12, 8), hole, holes.concat(wh).map(h => ({ p: h.p, r: [0, h.ry, 0], s: h.s })));
  const lay = m => { m.geometry.rotateX(H2); m.geometry.rotateY(-1.9); m.geometry.translate(-.42, .05, 1.42); return m; };
  const kb = lay(M(blade(.62, s => .075 * (1 - s * s * .75), s => .008 * (1 - s * .6)), steel)), kh = turned([[0, 0], [.045, 0], [.05, .1], [.042, .3], [.05, .36], [0, .38]], wd, { sm: 30, seg: 20 });
  kh.geometry.translate(0, -.38, 0); lay(kh);
  return item('Wheel of cheese', [part('rind', 'body', [0, 0, -.3], [wheel, wedge]), part('paste', 'trim', [0, .2, .5], faces.concat(wf, [dots])), part('knife', 'detail', [-.6, 0, .6], [kb, kh])], { stand: true, yaw: .45, el: 30 });
}

export function fish(c) {
  const skin = shell(c.body, { rough: .3, metal: .5, irid: .7, bump: 1.1 }), finM = matte(c.trim, { rough: .5, sheen: .6, double: true, wet: .3 }), cordM = matte(c.detail, { rough: .9, bump: .8 }), n = makeNoise(9, 16), L = 2.1;
  skin.vertexColors = true;
  // Half height and half thickness along the body, head at s = 0. The belly hangs a little lower than the back rises.
  const hh = s => .34 * Math.pow(Math.sin(Math.PI * Math.pow(Math.min(1, s * 1.04), .62)), .9) * (1 - s * .35) + .035, th = s => hh(s) * (.44 - s * .12);
  const at = (s, a) => V((s - .45) * L, Math.sin(a) * hh(s) * (Math.sin(a) < 0 ? 1.14 : 1) - .03 * Math.sin(s * 3), Math.cos(a) * th(s));
  // Dark along the back, pale under the belly, with a blush along the flank.
  const shade = a => { const k = Math.sin(a); return k > 0 ? 1 - k * .55 : 1 - k * 1.1; };
  const N = 80, A = 40, rings = [], col = [];
  for (let i = 0; i <= N; i++) { const s = i / N, pts = []; for (let j = 0; j < A; j++) pts.push(at(s, j / A * TAU)); rings.push(pts); }
  const bodyG = loft(rings); { const uv = bodyG.attributes.uv; for (let i = 0; i < uv.count; i++) { const k = shade(uv.getX(i) * TAU) * (1 + (n(uv.getX(i) * 40, uv.getY(i) * 30) - .5) * .25); col.push(k, k, k); } bodyG.setAttribute('color', new THREE.Float32BufferAttribute(col, 3)); }
  const body = M(bodyG, skin);
  // Scales are in the skin itself: rows of rounded plates, each overlapping the next toward the tail, none on the head.
  const NU = 34, NV = 46, plate = (u, v) => { const row = Math.floor(v * NV), x = (((u * NU + (row % 2) * .5) % 1) + 1) % 1 - .5, y = v * NV - row, d = Math.hypot(x * 2, y), k = Math.min(1, Math.max(0, (v - .17) / .05)); return .5 + ((d < 1 ? d : .15) - .5) * k; };
  skin.bumpMap = tex(1024, 1024, plate); skin.bumpScale = 2.2; skin.map = tex(1024, 1024, (u, v) => { const k = .72 + plate(u, v) * .4; return [k, k, k]; }, { srgb: true });
  const fin = (root, tip, spread, ribs, fork) => sheet((u, v) => { const a = (u - .5) * spread, d = v * (1 - .07 * Math.cos(u * Math.PI * ribs * 2)) * (fork ? .62 + .38 * Math.pow(Math.abs(u * 2 - 1), 1.4) : 1 - .25 * Math.pow(Math.abs(u * 2 - 1), 2)), base = root[0].clone().lerp(root[1], u), dir = tip.clone().applyAxisAngle(Z, a); return base.clone().addScaledVector(dir, d).add(V(0, 0, .012 * Math.cos(u * Math.PI * ribs * 2) * v + .02 * Math.sin(u * 5 + v * 3) * v)); }, 60, 10, finM, { uvx: 6, uvy: 2 });
  const tx = (1 - .45) * L, tail = fin([V(tx - .07, .04, th(1)), V(tx - .07, -.04, th(1))], V(.6, 0, 0), 1.5, 7, true);
  const dorsal = fin([V(-.16, hh(.38) - .03, th(.38) * .7), V(.46, hh(.68) - .03, th(.68) * .7)], V(.12, .26, 0), .7, 7), belly = fin([V(.26, -hh(.58) * 1.1, th(.58) * .7), V(.56, -hh(.72) * 1.08, th(.72) * .7)], V(.14, -.2, 0), .6, 4);
  const pect = fin([V(-.5, -.1, th(.2) + .006), V(-.47, -.2, th(.2) - .01)], V(.34, -.1, .05), .8, 5);
  const ex = -.79, ey = .075, ez = th(.075) * .93, eye = [ball(.052, metal('#d8b860', { kind: 'cast', rough: .2 }), { p: [ex, ey, ez], s: [1, 1, .45] }), ball(.034, dark(), { p: [ex, ey, ez + .014], s: [1, 1, .4] })];
  const gill = rod([[-.6, .2, th(.17) * .86], [-.51, .06, th(.19) + .006], [-.53, -.12, th(.19) + .002], [-.62, -.25, th(.17) * .8]], .011, dark(), { n: 16, sides: 6 }), mouth = rod([[-.955, -.035, .03], [-.9, -.04, th(.025) + .012], [-.85, -.055, th(.04) + .008]], .008, dark(), { n: 8, sides: 5 });
  // Strung through the gill for carrying, the way a catch comes back from the river.
  const cord = rod([[-.55, .0, th(.18) + .02], [-.7, .3, .2], [-.6, .62, .05], [-.2, .74, .02], [.3, .7, .02], [.62, .56, .02]], .016, cordM, { n: 40, sides: 7 }), knot = ball(.034, cordM, { p: [-.55, .0, th(.18) + .03] });
  return item('Fish', [part('body', 'body', [0, 0, 0], [body].concat(eye, [mouth, gill])), part('fins', 'trim', [.4, .2, .3], [tail, dorsal, belly, pect]), part('cord', 'detail', [-.3, .6, .3], [cord, knot])], { yaw: .35, fill: .92 });
}

export function honey(c) {
  const gl = glass('#dfe8e4', { tint: .12 }), gold = liquid(c.body, { lit: .3, inner: 1.7 }), cap = cloth(c.trim, { bump: .8, sheen: .5 }), wd = wood(c.detail, { bump: .8 }), twine = matte('#b8a47a', { rough: .9, bump: .8 }), n = makeNoise(18, 16);
  const jar = turned([[0, .04], [.3, .01], [.44, .04], [.5, .18], [.5, .8], [.44, .93], [.36, .98], [.36, 1.1], [.39, 1.12], [.39, 1.16], [.33, 1.17]], gl, { sm: 100, seg: 64 });
  const fill = turned([[0, .08], [.3, .05], [.42, .08], [.46, .2], [.46, .76], [.44, .8], [0, .8]], gold, { sm: 0, seg: 56 });
  // A cloth cover tied down with twine, its skirt standing out in folds below the string.
  const cover = sheet((u, v) => { const a = u * TAU, d = v * .64, flat = Math.min(d, .4), over = Math.max(0, d - .4), rr = flat + over * (.45 + .12 * Math.sin(a * 9 + 1)), y = 1.18 - over * (.82 + .1 * Math.sin(a * 7)); return V(Math.cos(a) * rr * (d < .4 ? 1 : 1 + over * .3), y + (n(a * 2, d * 4) - .5) * .012, Math.sin(a) * rr * (d < .4 ? 1 : 1 + over * .3)); }, 110, 22, cap, { uvx: 8, uvy: 3 });
  const string = [torus(.405, .014, twine, { r: [Math.PI / 2, 0, 0], p: [0, 1.085, 0] }), rod([[.4, 1.085, .06], [.5, 1.0, .12], [.52, .86, .14]], .012, twine, { n: 10, sides: 6 }), rod([[.4, 1.085, .06], [.46, 1.06, .2], [.42, .92, .24]], .012, twine, { n: 10, sides: 6 })];
  // The dipper leans against the jar with honey running off it.
  const dip = turned([[0, 0], [.07, .01], [.1, .05], [.1, .3], [.07, .34], [.03, .36], [.026, 1.05], [.04, 1.1], [0, 1.12]], wd, { sm: 0, seg: 24 }), grooves = [.08, .14, .2, .26].map(y => torus(.1, .012, matte('#3a2616'), { r: [Math.PI / 2, 0, 0], p: [0, y, 0] }));
  for (const m of [dip].concat(grooves)) { m.geometry.rotateZ(-.9); m.geometry.translate(.62, .1, .5); }
  const drip = [blob([.16, .02, .12], v => 1 + (n(v.x * 3, v.z * 3) - .5) * .4, gold, { p: [.68, .012, .52] }), rod([[.66, .2, .5], [.67, .1, .51], [.68, .03, .52]], s => .03 - .014 * Math.sin(s * Math.PI), gold, { n: 10, sides: 8 })];
  // A piece of comb broken from the frame: six-sided cells in its face, most of them full, a few capped over.
  const cell = (u, v) => { const S = 9, x = u * S, y = v * S / .866, row = Math.round(y), cx = Math.round(x - (row % 2) * .5) + (row % 2) * .5, dx = Math.abs(x - cx), dy = Math.abs(y - row) * .866, d = Math.max(dx * .866 + dy * .5, dy), capped = n(cx * 3.1 + 5, row * 2.7) > .72; return [d, capped]; };
  const combM = matte(c.body, { rough: .5, wet: .5, bump: 3, double: true }); combM.map = tex(512, 512, (u, v) => { const [d, cap] = cell(u, v); return d > .4 ? [1, .96, .72] : cap ? [.98, .88, .6] : [.62 + d * .5, .4 + d * .5, .1 + d * .4]; }, { srgb: true }); combM.bumpMap = tex(512, 512, (u, v) => { const [d, cap] = cell(u, v); return d > .4 ? 1 : cap ? .85 : .2 + d; });
  const edgeM = wax('#' + new THREE.Color(c.body).lerp(new THREE.Color('#fff0b8'), .5).getHexString(), { lit: .05 });
  const chunk = rbox(.86, .2, .56, .05, edgeM, { p: [0, .1, 0] }), top = sheet((u, v) => V((u - .5) * .78, .203, (v - .5) * .48), 2, 2, combM);
  const ooze = [blob([.2, .016, .13], v => 1 + (n(v.x * 3 + 4, v.z * 3) - .5) * .5, gold, { p: [.4, .012, .3] }), rod([[.36, .17, .2], [.42, .1, .27], [.44, .03, .3]], s => .03 - .014 * Math.sin(s * Math.PI), gold, { n: 10, sides: 8 })];
  const comb = [chunk, top].concat(ooze);
  for (const m of comb) { m.geometry.rotateY(.5); m.geometry.translate(-.86, 0, .6); }
  return item('Jar of honey', [part('jar', '', [0, 0, 0], [jar]), part('honey', 'body', [0, 0, .7], [fill].concat(drip, comb)), part('cover', 'trim', [0, .5, 0], [cover].concat(string)), part('dipper', 'detail', [.6, .3, .3], [dip].concat(grooves))], { stand: true, yaw: .2, el: 20 });
}

export function lockpicks(c) {
  const hide = leather(c.body, { bump: 1.3, double: true }), steel = metal(c.trim, { kind: 'steel', rough: .28 }), thread = matte(c.detail, { rough: .8, bump: .5 }), n = makeNoise(22, 16), W = 2.0, Hh = 1.2;
  // A leather roll lying open, one end still curled the way it was rolled.
  const roll = sheet((u, v) => { const x = (u - .5) * W, y = (v - .5) * Hh, t = Math.max(0, u - .82) / .18, a = t * 3.6, cx = (.82 - .5) * W; return V(u > .82 ? cx + Math.sin(a) * .11 : x, y + (n(u * 5, v * 5) - .5) * .012, (u > .82 ? .11 - Math.cos(a) * .11 : 0) + .012 + (n(u * 4 + 5, v * 4) - .5) * .01); }, 80, 16, hide, { uvx: 4, uvy: 3 });
  const pocket = rbox(1.5, .34, .02, .008, hide, { p: [-.14, -.3, .028] });
  const stitch = [], st = (x0, x1, y) => { for (let x = x0; x < x1; x += .055) stitch.push({ p: [x, y, .04], s: [.022, .006, .005] }); };
  st(-.88, .6, -.14); st(-.88, .6, -.46); for (let i = 0; i <= 6; i++) for (let y = -.44; y < -.15; y += .055) stitch.push({ p: [-.89 + i * .25, y, .04], s: [.006, .022, .005] });
  // Six tools, each a flat steel shank with a different working end.
  const tips = [s => [0, 0], s => [.03 * Math.sin(s * 22), 0], s => [s > .86 ? (s - .86) * .5 : 0, 0], s => [.022 * Math.sin(s * 34), 0], s => [s > .8 ? -(s - .8) * .45 : 0, 0], s => [0, 0]], tools = [];
  for (let i = 0; i < 6; i++) { const x = -.765 + i * .25, pts = []; for (let k = 0; k <= 30; k++) { const s = k / 30, d = s > .62 ? tips[i]((s - .62) / .38 * .5 + .5) : [0, 0]; pts.push(V(x + d[0], -.52 + s * 1.0, .05 + (s < .28 ? 0 : .012))); } tools.push(M(tube(pts, s => s < .5 ? .017 : .017 - (s - .5) * .02, 6), steel), rbox(.05, .3, .018, .006, steel, { p: [x, -.4, .05] })); if (i === 5) tools.push(rbox(.1, .028, .018, .005, steel, { p: [x + .035, .47, .062] })); }
  const cord = rod([[1.0, 0, .2], [1.12, -.2, .1], [1.2, -.5, .02], [1.05, -.72, .02], [.8, -.76, .02]], .018, thread, { n: 30, sides: 7 }), cord2 = rod([[1.0, 0, .2], [1.16, .2, .08], [1.14, .5, .02], [.92, .7, .02]], .018, thread, { n: 30, sides: 7 });
  return item('Lockpicks', [part('roll', 'body', [0, 0, -.3], [roll, pocket]), part('picks', 'trim', [0, .3, .6], tools), part('stitching and ties', 'detail', [.4, -.3, .3], [many(new THREE.SphereGeometry(1, 6, 4), thread, stitch), cord, cord2])], { yaw: .3, fill: .92 });
}
