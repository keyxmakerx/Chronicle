// Goods: things traded, stored and played.
import { THREE, V, M, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, slab, circle, blob, sheet, fan, rim, many, band, drawn, cutStone, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, gem, bone, paper, matte, dark } from '../mats.js';

export function ingot(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .3, bump: .9 });
  const bar = (p, ry) => { const m = slab([[-.3, 0], [.3, 0], [.22, .2], [-.22, .2]], 1.0, .02, gold, { bseg: 3 }); m.geometry.rotateY(ry); m.geometry.translate(p[0], p[1], p[2]); return m; };
  const stamp = (p, ry) => { const m = torus(.07, .012, gold, { r: [Math.PI / 2, 0, 0], p: [0, .2, 0] }); m.geometry.rotateY(ry); m.geometry.translate(p[0], p[1], p[2]); return m; };
  const A = [[-.34, 0, 0], .06], B = [[.34, 0, .04], -.05], C = [[0, .2, 0], .5];
  return item('Ingots', [part('bar', 'body', [-.4, 0, 0], [bar(A[0], A[1]), stamp(A[0], A[1])]), part('bar', 'body', [.4, 0, 0], [bar(B[0], B[1]), stamp(B[0], B[1])]), part('bar', 'body', [0, .5, 0], [bar(C[0], C[1]), stamp(C[0], C[1])])], { stand: true, yaw: .5, el: 26 });
}

export function coins(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .26 }), silver = metal(c.trim, { kind: 'cast', rough: .26 }), copper = metal(c.detail, { kind: 'cast', rough: .34 }), r = rng(13);
  const coin = (mat, R) => { const m = turned([[0, .02], [R * .55, .02], [R * .6, .03], [R * .8, .03], [R * .84, .02], [R * .9, .02], [R * .93, .04], [R, .04], [R, 0], [0, 0]], mat, { sm: 0, seg: 40 }); return m; };
  const put = (m, p, rot) => { if (rot) { m.geometry.rotateX(rot[0]); m.geometry.rotateY(rot[1]); m.geometry.rotateZ(rot[2]); } m.geometry.translate(p[0], p[1], p[2]); return m; };
  const stack = []; for (let i = 0; i < 9; i++) stack.push(put(coin(gold, .3), [(r() - .5) * .05, i * .042, (r() - .5) * .05], [0, r() * 6, 0]));
  const stack2 = []; for (let i = 0; i < 4; i++) stack2.push(put(coin(gold, .3), [.62 + (r() - .5) * .05, i * .042, -.2 + (r() - .5) * .05], [0, r() * 6, 0]));
  const loose = [put(coin(silver, .26), [.5, .0, .42], [0, 1, 0]), put(coin(silver, .26), [-.52, .0, .34], [0, 2, 0]), put(coin(silver, .26), [.3, .12, .05], [0, 0, 1.15])];
  const small = [put(coin(copper, .19), [-.2, 0, .6], [0, .4, 0]), put(coin(copper, .19), [.12, 0, .7], [0, 1.4, 0]), put(coin(copper, .19), [-.6, 0, -.1], [0, 2.2, 0]), put(coin(copper, .19), [.02, .04, .62], [.25, 3, 0])];
  return item('Coins', [part('stack', 'body', [0, .3, 0], stack.concat(stack2)), part('loose coin', 'trim', [.4, 0, .4], loose), part('small change', 'detail', [-.4, 0, .5], small)], { stand: true, yaw: .3, el: 28 });
}

export function chest(c) {
  const wd = wood(c.body, { bump: 1.2 }), iron = metal(c.trim, { kind: 'forged', rough: .5 }), gold = metal(c.detail, { kind: 'cast' }), W = 1.5, H = .62, Dp = .86;
  const box = rbox(W, H, Dp, .03, wd, { p: [0, H / 2, 0] });
  const lidG = new THREE.CylinderGeometry(Dp / 2, Dp / 2, W, 40, 1, false, 0, Math.PI); lidG.rotateZ(Math.PI / 2); lidG.scale(1, .82, 1);
  const lid = M(lidG, wd, { p: [0, H + .02, 0] });
  const strap = x => { const g = new THREE.CylinderGeometry(Dp / 2 + .012, Dp / 2 + .012, .12, 40, 1, true, 0, Math.PI); g.rotateZ(Math.PI / 2); g.scale(1, .82, 1); return [M(g, metal(c.trim, { kind: 'forged', rough: .5, double: true }), { p: [x, H + .02, 0] }), rbox(.12, H + .02, .024, .008, iron, { p: [x, H / 2, Dp / 2 + .006] }), rbox(.12, H + .02, .024, .008, iron, { p: [x, H / 2, -Dp / 2 - .006] })]; };
  const straps = [-.5, .5].flatMap(strap);
  const edge = [rbox(W + .03, .05, Dp + .03, .012, iron, { p: [0, H + .005, 0] }), rbox(W + .03, .05, Dp + .03, .012, iron, { p: [0, .03, 0] })];
  const studs = many(new THREE.SphereGeometry(.022, 8, 6), iron, [-.5, .5].flatMap(x => [.12, .3, .5].map(y => ({ p: [x, y, Dp / 2 + .02] }))));
  const lock = [rbox(.2, .26, .04, .015, gold, { p: [0, H - .05, Dp / 2 + .02] }), M(new THREE.CircleGeometry(.03, 16), dark(), { p: [0, H - .03, Dp / 2 + .042] }), rbox(.022, .07, .002, .001, dark(), { p: [0, H - .08, Dp / 2 + .042] }), rbox(.12, .1, .03, .01, gold, { p: [0, H + .09, Dp / 2 + .02] })];
  const rings = [-1, 1].map(k => torus(.09, .016, iron, { r: [0, Math.PI / 2, 0], p: [k * (W / 2 + .03), H * .62, 0] }));
  return item('Chest', [part('box', 'body', [0, -.2, 0], [box]), part('lid', 'body', [0, .5, 0], [lid]), part('bands', 'trim', [0, 0, .5], straps.concat(edge, [studs], rings)), part('lock', 'detail', [0, 0, 1], lock)], { stand: true, yaw: .55, el: 18 });
}

export function lute(c) {
  const top = wood(c.body, { polish: .5, bump: .3 }), dk = wood(c.trim, { polish: .5 }), bn = bone(c.detail), gut = matte(c.detail, { rough: .5 });
  const out = [[0, .28], [.26, .2], [.52, -.05], [.66, -.45], [.6, -.9], [.36, -1.18], [0, -1.28], [-.36, -1.18], [-.6, -.9], [-.66, -.45], [-.52, -.05], [-.26, .2]];
  const face = fan(out, () => .3, top, { cy: -.55, nu: 120, nv: 16, uvx: 1, uvy: 3 });
  const backM = wood(c.trim, { polish: .5 }); backM.side = THREE.DoubleSide;
  const back = fan(out, (x, y, v) => .3 * Math.pow(v, 6) + .0, backM, { cy: -.55, nu: 120, nv: 20 });
  const edge = rim(out, () => .3, .014, dk, { n: 160 });
  const rose = [M(new THREE.CircleGeometry(.17, 40), dark(), { p: [0, -.38, .302] }), torus(.17, .014, dk, { p: [0, -.38, .304] }), torus(.11, .008, bn, { p: [0, -.38, .304] })].concat([0, 1, 2].map(i => rbox(.33, .012, .006, .002, bn, { r: [0, 0, i / 3 * Math.PI], p: [0, -.38, .304] })));
  const neck = slab([[-.1, .2], [.1, .2], [.075, 1.5], [-.075, 1.5]], .1, .04, dk, { p: [0, 0, .27] }), board = slab([[-.095, .16], [.095, .16], [.07, 1.5], [-.07, 1.5]], .02, .006, wood('#2a1a14', { polish: .6 }), { p: [0, 0, .33] });
  const frets = many(new THREE.CylinderGeometry(.006, .006, 1, 6).rotateZ(Math.PI / 2), gut, [.4, .58, .75, .9, 1.04, 1.17, 1.29, 1.4].map(y => ({ p: [0, y, .342], s: [.2 - (y - .16) * .037, 1, 1] })));
  const head = slab([[-.085, 0], [.085, 0], [.06, .62], [-.06, .62]], .09, .03, dk, { r: [-1.1, 0, 0], p: [0, 1.5, .3] });
  const pegs = []; for (let i = 0; i < 6; i++) { const k = i % 2 ? 1 : -1, t = .1 + Math.floor(i / 2) * .19; pegs.push(turned([[.02, 0], [.016, .1], [.03, .14], [.034, .2], [0, .22]], bn, { sm: 12, seg: 10, r: [0, 0, -k * Math.PI / 2], p: [k * .07, 1.5 + t * Math.cos(1.1), .3 - t * Math.sin(1.1)] })); }
  const bridge = rbox(.42, .05, .03, .01, dk, { p: [0, -.98, .315] });
  const strings = [-2.5, -1.5, -.5, .5, 1.5, 2.5].map(i => rod([[i * .06, -.98, .335], [i * .05, .3, .346], [i * .022, 1.5, .348]], .0045, gut, { sides: 5 }));
  return item('Lute', [part('bowl', 'body', [0, -.3, -.2], [face, back, edge]), part('rose', 'detail', [0, -.3, .6], rose), part('neck', 'trim', [0, .4, 0], [neck, board, frets]), part('head', 'trim', [0, .8, 0], [head]), part('pegs', 'detail', [.5, .8, 0], pegs), part('bridge', 'trim', [0, -.7, .6], [bridge]), part('strings', 'detail', [0, 0, 1], strings)], { yaw: -.6, fill: .95 });
}

export function tome(c) {
  const hide = leather(c.body, { bump: 1.4 }), br = metal(c.trim, { kind: 'cast' }), st = gem(c.detail, { round: true, edges: 0 }), W = 1.0, H = 1.36, T = .3;
  const lines = drawn(64, 256, (x, w, h) => { for (let i = 0; i < 90; i++) { x.fillStyle = 'rgba(90,70,40,' + (.15 + (i * 7 % 5) * .06) + ')'; x.fillRect(0, i * 3, w, 1); } }, [.93, .89, .78]);
  const pages = rbox(W - .06, H - .08, T - .07, .01, paper('#ffffff', { map: lines }), { p: [.02, 0, 0] });
  const front = rbox(W, H, .035, .014, hide, { p: [0, 0, T / 2 - .017] }), backC = rbox(W, H, .035, .014, hide, { p: [0, 0, -T / 2 + .017] });
  const spine = M(new THREE.CylinderGeometry(T / 2, T / 2, H, 24, 1, false, Math.PI, Math.PI), hide, { s: [.5, 1, 1], p: [-W / 2 + .005, 0, 0] });
  const ribs = [-.42, -.14, .14, .42].map(y => M(new THREE.TorusGeometry(T / 2 + .004, .016, 8, 20, Math.PI), hide, { r: [Math.PI / 2, 0, Math.PI / 2], s: [.5, 1, 1], p: [-W / 2 + .005, y, 0] }));
  const corners = [[1, 1], [1, -1], [-1, 1], [-1, -1]].map(([sx, sy]) => { const m = slab([[0, 0], [-.24, 0], [-.24, -.06], [-.06, -.24], [0, -.24]], .014, .005, br); m.geometry.scale(sx, sy, 1); m.geometry.translate(sx * (W / 2 + .004), sy * (H / 2 + .004), T / 2 + .004); return m; });
  const boss = [turned([[0, .035], [.1, .035], [.14, .02], [.15, 0]], br, { sm: 16, r: [Math.PI / 2, 0, 0], p: [0, 0, T / 2] }), ball(.085, st, { s: [1, 1, .6], p: [0, 0, T / 2 + .04] })];
  const frame = [rbox(.5, .02, .012, .004, br, { p: [0, .3, T / 2 + .004] }), rbox(.5, .02, .012, .004, br, { p: [0, -.3, T / 2 + .004] }), rbox(.02, .6, .012, .004, br, { p: [.25, 0, T / 2 + .004] }), rbox(.02, .6, .012, .004, br, { p: [-.25, 0, T / 2 + .004] })];
  const clasp = [rbox(.16, .12, .014, .005, br, { p: [W / 2 - .06, .0, T / 2 + .006] }), rbox(.02, .12, T + .02, .006, br, { p: [W / 2 + .012, 0, 0] })];
  return item('Tome', [part('pages', '', [.3, 0, 0], [pages]), part('cover', 'body', [0, 0, 0], [front, backC, spine].concat(ribs)), part('corners and clasp', 'trim', [0, 0, .5], corners.concat(frame, clasp)), part('boss', 'detail', [0, 0, 1], boss)], { yaw: .4, fill: .9 });
}
