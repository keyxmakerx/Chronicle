// More tools and gear: a shovel, a saw, a fishing rod, a spyglass, a bedroll, a waterskin, a grappling hook, balance scales and a bear trap; and a hand mirror, a chalice and a bell.
import { THREE, V, M, X, Y, Z, part, item, turned, smooth, rod, ball, torus, cyl, rbox, slab, shape, hole, circle, blob, sheet, many, aim, band, wrap, bar, fan, rim, chain, cutStone, strands, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, matte, gem, glass, liquid, dark } from '../mats.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
const sstep = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
const fine = (m, n) => { for (const k of ['map', 'bumpMap']) if (m[k]) { m[k] = m[k].clone(); m[k].repeat.set(n, n); m[k].needsUpdate = true; } return m; };

export function shovel(c) {
  const steel = metal(c.body, { kind: 'forged', rough: .4, double: true }), wd = wood(c.trim), cord = leather(c.detail);
  const shaft = turned([[.045, -1.2], [.042, 0], [.046, .98]], wd, { sm: 0, seg: 24 });
  // The blade: a pointed spade, dished a little, with a rib where the socket runs into it.
  const out = [[0, 2.02], [.16, 1.86], [.3, 1.6], [.34, 1.3], [.33, 1.06], [.26, 1.0], [0, .98], [-.26, 1.0], [-.33, 1.06], [-.34, 1.3], [-.3, 1.6], [-.16, 1.86]];
  const zf = (x, y) => -.09 * (1 - Math.pow(Math.abs(x) / .36, 2)) + .06 * Math.exp(-Math.pow(x / .05, 2)) * (1 - sstep(1.0, 1.7, y));
  const bladeM = fan(out, zf, steel, { nu: 120, nv: 20, cy: 1.4, tension: .4 }), edge = rim(out, zf, .012, steel, { n: 160, tension: .4 });
  const socket = turned([[.046, .7], [.056, .74], [.05, .98], [.036, 1.2], [.01, 1.36]], steel, { sm: 30, seg: 24, p: [0, 0, -.03] });
  const treads = [-1, 1].map(k => rbox(.2, .03, .05, .012, steel, { p: [k * .2, 1.02, -.05] }));
  // A D handle: the shaft forks, and a turned grip closes the fork.
  const fork = [-1, 1].map(k => rod([[0, -1.16, 0], [k * .1, -1.3, 0], [k * .17, -1.5, 0], [k * .17, -1.62, 0]], .034, wd, { n: 16, sides: 10 }));
  const grip = turned([[.03, -.2], [.04, -.14], [.044, 0], [.04, .14], [.03, .2]], wd, { sm: 30, seg: 20, r: [0, 0, H2], p: [0, -1.62, 0] });
  const bind = wrap(-1.02, -1.18, () => .047, cord, { turns: 8, cord: .012 }), bind2 = wrap(.56, .72, () => .047, cord, { turns: 8, cord: .012 });
  return item('Shovel', [part('shaft and handle', 'trim', [0, -.3, 0], [shaft, grip].concat(fork)), part('blade', 'body', [0, .4, 0], [bladeM, edge, socket].concat(treads)), part('bindings', 'detail', [.3, 0, .3], bind.concat(bind2))], { yaw: -.3, fill: .97 });
}

export function saw(c) {
  const steel = metal(c.body, { rough: .3 }), wd = wood(c.trim, { polish: .3 }), br = metal(c.detail, { kind: 'cast' });
  // The blade runs up from the handle, narrowing; the teeth are on the left edge.
  const pts = [[.3, -.5], [.16, 1.9], [-.06, 1.9]], N = 44;
  for (let i = 0; i <= N; i++) { const s = i / N, y = 1.9 - s * 2.34, x = -.06 - s * .3; pts.push([x, y]); if (i < N) pts.push([x - .045, y - 2.34 / N * .5]); }
  pts.push([-.36, -.5]);
  const bladeM = slab(pts, .022, .006, steel, { bseg: 1 });
  // The handle: a closed grip sawn from one board.
  const hs = hole(shape([[-.44, -.44], [-.3, -.38], [.36, -.36], [.5, -.5], [.52, -.8], [.44, -1.14], [.2, -1.3], [-.2, -1.28], [-.42, -1.1], [-.5, -.8]], true), [[-.26, -.66], [.2, -.62], [.3, -.8], [.22, -1.02], [-.12, -1.06], [-.3, -.9]], true);
  const handle = slab(hs, .1, .04, wd, { bseg: 5 });
  const screws = [[-.26, -.5], [0, -.48], [.26, -.5]].map(([x, y]) => [-1, 1].map(k => turned([[0, .014], [.03, .012], [.04, 0]], br, { sm: 0, seg: 16, r: [k * H2, 0, 0], p: [x, y, k * .05] }))).flat();
  return item('Saw', [part('blade', 'body', [0, .4, 0], [bladeM]), part('handle', 'trim', [0, -.3, 0], [handle]), part('screws', 'detail', [0, -.1, .5], screws)], { yaw: .45, fill: .96 });
}

export function fishingrod(c) {
  const cane = wood(c.body, { polish: .5, bump: .5 }), br = metal(c.trim, { kind: 'cast', rough: .26 }), red = matte(c.detail, { rough: .5, wet: .3 }), line = matte('#e8e2d0', { rough: .6 }), cork = matte('#b89a6a', { rough: .9, bump: .6 });
  // The rod bends a little under its own line.
  const sp = Array.from({ length: 61 }, (_, i) => { const s = i / 60; return V(.5 * Math.pow(s, 2.4), -1.2 + s * 3.4, 0); });
  const rodM = M(tube(sp, s => .04 - .032 * s, 10), cane);
  const whips = [.12, .3, .46, .6, .72, .82, .9].map(s => { const p = sp[Math.round(s * 60)]; return torus(.045 - .032 * s, .009, red, { r: [H2, 0, 0], p: [p.x, p.y, p.z] }); });
  const guides = [.3, .5, .68, .82, .93, 1].map(s => { const p = sp[Math.round(s * 60)]; return torus(.034 - .012 * s, .006, br, { r: [H2, 0, 0], p: [p.x - (.07 - .03 * s), p.y, p.z] }); });
  const handle = turned([[.03, -1.9], [.056, -1.86], [.06, -1.6], [.05, -1.3], [.056, -1.2], [.045, -1.16]], cork, { sm: 40, seg: 24 }), butt = turned([[0, -1.95], [.05, -1.93], [.06, -1.88], [.05, -1.85]], br, { sm: 16, seg: 20 });
  // A brass reel under the handle.
  const reel = [cyl(.15, .15, .03, br, { r: [0, 0, H2], p: [-.12, -1.12, .0] }), cyl(.15, .15, .03, br, { r: [0, 0, H2], p: [-.22, -1.12, 0] }), cyl(.09, .09, .08, line, { r: [0, 0, H2], p: [-.17, -1.12, 0] }), rbox(.06, .3, .05, .012, br, { p: [-.065, -1.12, 0] })];
  const crank = [rod([[-.25, -1.12, 0], [-.3, -1.12, 0], [-.3, -1.04, .08]], .012, br, { sides: 8 }), ball(.03, red, { p: [-.33, -1.03, .09], s: [1.6, 1, 1] })];
  // The line runs up through the guides and hangs from the tip with a float and a hook.
  const lp = [V(-.17, -1.03, 0)].concat([.3, .5, .68, .82, .93, 1].map(s => { const p = sp[Math.round(s * 60)]; return V(p.x - (.07 - .03 * s), p.y, p.z); }), [V(.52, 1.7, 0), V(.56, 1.2, 0), V(.56, .62, 0)]);
  const lineM = rod(lp, .005, line, { sides: 5 });
  const float = [turned([[0, .16], [.012, .15], [.05, .06], [.06, 0], [.04, -.07], [.012, -.14], [0, -.15]], red, { sm: 30, seg: 20, p: [.56, 1.1, 0] }), turned([[.052, .05], [.062, 0], [.046, -.055]], matte('#f2ead8'), { sm: 12, seg: 20, p: [.56, 1.1, 0] })];
  const hook = rod([[.56, .62, 0], [.56, .5, 0], [.58, .42, 0], [.63, .4, 0], [.66, .45, 0], [.65, .5, 0]], s => .011 * (1 - s * .6), br, { n: 20, sides: 6 });
  return item('Fishing rod', [part('rod', 'body', [0, 0, 0], [rodM, handle]), part('reel and guides', 'trim', [-.3, -.2, .3], reel.concat(guides, [butt, crank[0], hook])), part('float and whippings', 'detail', [.3, .2, .3], float.concat(whips, [crank[1], lineM]))], { yaw: .25, fill: .98 });
}

export function spyglass(c) {
  const hide = leather(c.body, { bump: 1.2 }), br = metal(c.trim, { kind: 'cast', rough: .24 }), lens = glass(c.detail, { tint: .35 });
  // Three brass draws pulled out of a barrel bound in hide; the big lens is at the top.
  const draw = (r, y0, y1) => turned([[r - .012, y0], [r, y0], [r, y1], [r - .012, y1]], br, { sm: 0, seg: 40 });
  const draws = [draw(.085, -1.5, -.86), draw(.11, -.9, -.26), draw(.135, -.3, .3)];
  const collars = [[.095, -1.5, .06], [.12, -.88, .05], [.145, -.28, .05], [.19, .32, .08], [.2, 1.46, .1]].map(([r, y, w]) => band(r - .008, y, w, .016, br));
  const barrel = turned([[.16, .3], [.168, .5], [.175, 1.44]], hide, { sm: 0, seg: 40 });
  const eye = turned([[.03, -1.62], [.07, -1.6], [.1, -1.56], [.1, -1.5], [.085, -1.5]], br, { sm: 0, seg: 32 }), pupil = cyl(.03, .03, .01, dark(), { p: [0, -1.622, 0] });
  const hood = turned([[.188, 1.42], [.2, 1.5], [.2, 1.58], [.185, 1.58], [.18, 1.5]], br, { sm: 0, seg: 40 });
  const pane = turned([[0, 1.53], [.1, 1.525], [.182, 1.5], [.1, 1.49], [0, 1.488]], lens, { sm: 16, seg: 40 }), inside = cyl(.176, .176, .02, dark(), { p: [0, 1.46, 0] });
  const stitch = many(new THREE.CapsuleGeometry(.006, .03, 4, 6), matte('#d8c8a0'), Array.from({ length: 24 }, (_, i) => ({ p: [0, .36 + i * .045, .176], r: [0, 0, i % 2 ? .5 : -.5] })));
  return item('Spyglass', [part('barrel', 'body', [0, .4, 0], [barrel, stitch]), part('draws', 'trim', [0, -.3, 0], draws.concat(collars, [eye, pupil, hood, inside])), part('lens', 'detail', [0, .8, 0], [pane])], { yaw: .4, fill: .97 });
}

export function bedroll(c) {
  const wool = cloth(c.body, { bump: 1, sheen: .4 }), hide = leather(c.trim), stripe = cloth(c.detail, { bump: 1, sheen: .4 }), br = metal('#b08a3a', { kind: 'cast' }), n = makeNoise(7, 16), R = .44, L = .95;
  // Rolled up tight and strapped, lying on its side.
  const roll = sheet((u, v) => { const a = u * TAU, x = (v - .5) * 2 * L, r = R * (1 + (n(a * 2, x * 2) - .5) * .05 - .04 * Math.exp(-Math.pow((Math.abs(x) - .5) / .06, 2))); return V(x, R + Math.sin(a) * r, Math.cos(a) * r); }, 80, 40, wool, { uvx: 6, uvy: 4 });
  // Each end shows the blanket wound round itself.
  const spiral = k => { const pts = []; for (let i = 0; i <= 150; i++) { const s = i / 150, a = s * TAU * 4.2, r = .04 + s * (R - .05); pts.push(V(k * (L + .012 - .03 * (1 - s)), R + Math.sin(a) * r, Math.cos(a) * r)); } return pts; };
  const ends = [-1, 1].map(k => [M(tube(spiral(k), .045, 8), wool), cyl(R * .96, R * .96, .02, wool, { r: [0, 0, H2], p: [k * (L - .03), R, 0] })]).flat();
  const stripes = [-.78, -.64, .64, .78].map(x => sheet((u, v) => { const a = u * TAU, r = R * 1.012 * (1 + (n(a * 2, x * 2) - .5) * .05); return V(x + (v - .5) * .09, R + Math.sin(a) * r, Math.cos(a) * r); }, 80, 2, stripe));
  const straps = [-.5, .5].map(x => sheet((u, v) => { const a = u * TAU, r = R * .975; return V(x + (v - .5) * .1, R + Math.sin(a) * r, Math.cos(a) * r); }, 80, 2, hide));
  const buckles = [-.5, .5].map(x => [slab(hole(shape([[-.08, -.07], [.08, -.07], [.08, .07], [-.08, .07]]), [[-.05, -.04], [.05, -.04], [.05, .04], [-.05, .04]]), .03, .01, br, { r: [-.5, 0, 0], p: [x, R + Math.sin(1.07) * R * .99, Math.cos(1.07) * R * .99] }), sheet((u, v) => V(x + (u - .5) * .08, R + Math.sin(1.07) * R - v * .24, Math.cos(1.07) * R + .03 + .1 * v + .03 * Math.sin(v * 3)), 3, 8, hide)]).flat();
  // A carrying strap between the two.
  const carry = rod([[-.5, R * 2 - .03, 0], [-.3, R * 2 + .16, .02], [0, R * 2 + .24, .03], [.3, R * 2 + .16, .02], [.5, R * 2 - .03, 0]], .03, hide, { n: 30, sides: 8 });
  return item('Bedroll', [part('blanket', 'body', [0, 0, 0], [roll].concat(ends)), part('straps', 'trim', [0, .4, .3], straps.concat(buckles, [carry])), part('stripes', 'detail', [0, 0, .5], stripes)], { stand: true, yaw: .6, el: 18, fill: .9 });
}

export function waterskin(c) {
  const hide = leather(c.body, { bump: 1.4 }), horn = wood(c.trim, { polish: .5 }), cord = cloth(c.detail, { sheen: .3, bump: 1 }), n = makeNoise(19, 16);
  // A whole skin sewn up: fat at the bottom, drawn in to a neck, sagging to one side.
  const P = smooth([[0, -.8], [.3, -.76], [.5, -.52], [.55, -.2], [.47, .12], [.3, .4], [.16, .6], [.11, .72]], 60), lean = y => .16 * Math.pow(Math.max(0, (y + .1) / .9), 2);
  const skin = turned(P, hide, { sm: 0, seg: 64, s: [1, 1, .64] });
  { const p = skin.geometry.attributes.position; for (let i = 0; i < p.count; i++) { const y = p.getY(i), x = p.getX(i), z = p.getZ(i); p.setXYZ(i, (x + lean(y)) * (1 + (n(x * 3 + 3, y * 3 + z * 2) - .5) * .08), y, z * (1 + (n(x * 3, y * 3 + 5) - .5) * .1)); } skin.geometry.computeVertexNormals(); }
  const top = lean(.72), neck = turned([[.11, .7], [.1, .82], [.12, .9], [.13, .94], [.1, .96]], hide, { sm: 20, seg: 24, p: [top, 0, 0] });
  const collar = band(.1, .84, .06, .016, cord, { p: [top, 0, 0] });
  const stopper = turned([[.085, .95], [.09, 1.02], [.12, 1.06], [.125, 1.12], [.08, 1.17], [0, 1.18]], horn, { sm: 30, seg: 24, p: [top, 0, 0] });
  // The seam down one side, with its stitches.
  const side = P.filter(q => q[1] > -.74 && q[1] < .7), seam = rod(side.map(q => V(-q[0] + lean(q[1]) - .012, q[1], 0)), .024, hide, { sides: 8 });
  const st = side.filter((_, i) => i % 2 === 0).map(q => ({ p: [-q[0] + lean(q[1]) - .014, q[1], .028], r: [0, 0, .7] }));
  const stitches = many(new THREE.CapsuleGeometry(.007, .04, 4, 6), cord, st);
  // The strap: tied at the neck and at the foot, slack between.
  const strap = rod([[top, .86, .1], [.6, .96, .2], [.96, .5, .22], [.9, -.2, .2], [.5, -.78, .16], [.06, -.84, .06]], .022, cord, { n: 50, sides: 8 }), foot = ball(.07, hide, { p: [0, -.82, 0], s: [1.2, .8, 1] }), tie = torus(.07, .02, cord, { r: [H2, 0, 0], p: [0, -.8, 0] });
  return item('Waterskin', [part('skin', 'body', [0, 0, 0], [skin, neck, seam, foot]), part('stopper', 'trim', [.2, .5, 0], [stopper]), part('strap and stitches', 'detail', [.4, 0, .3], [strap, collar, tie, stitches])], { yaw: .2, fill: .9 });
}

export function grapple(c) {
  const iron = metal(c.body, { kind: 'forged', rough: .48 }), hemp = fine(cloth(c.trim, { sheen: .2, bump: 1.2 }), 3), whip = cloth(c.detail, { sheen: .3 }), r = rng(3);
  const shank = turned([[.05, .1], [.045, .9], [.055, 1.0], [.04, 1.06]], iron, { sm: 0, seg: 16 }), eye = torus(.1, .03, iron, { p: [0, 1.14, 0] });
  // Four claws leave the foot of the shank, sweep out and turn back up to a point.
  const claws = [0, 1, 2, 3].map(i => { const a = i / 4 * TAU + .5, cx = Math.cos(a), cz = Math.sin(a); return rod([[0, .12, 0], [cx * .2, -.06, cz * .2], [cx * .5, -.1, cz * .5], [cx * .72, .1, cz * .72], [cx * .78, .42, cz * .78]], s => .05 * (1 - Math.pow(s, 2) * .78) + .004, iron, { n: 30, sides: 10 }); });
  const boss = ball(.09, iron, { p: [0, .08, 0], s: [1, .8, 1] }), collar = band(.05, .2, .08, .018, iron);
  // The rope: knotted through the eye, then coiled.
  const rp = [V(.02, 1.22, 0), V(.2, 1.36, .02), V(.5, 1.2, .04), V(.66, .8, .04)];
  for (let i = 0; i <= 90; i++) { const s = i / 90, a = 1.2 + s * TAU * 2.6; rp.push(V(.98 + Math.cos(a) * (.34 - .03 * s), .34 + Math.sin(a) * (.4 - .03 * s), .06 + s * .12)); }
  const rope = M(tube(path(rp, 300), .034, 9), hemp);
  const knot = ball(.07, hemp, { p: [.02, 1.24, 0], s: [1.1, .9, 1] });
  const end = rp[rp.length - 1], whipM = torus(.036, .014, whip, { p: [end.x, end.y, end.z], r: [0, H2, 0] }), binds = [0, 1].map(k => torus(.08, .016, whip, { p: [.98, k ? .74 : -.06, .12], r: [H2, 0, 0], s: [1, 1.5, 1] }));
  return item('Grappling hook', [part('hook', 'body', [-.2, 0, 0], [shank, eye, boss, collar].concat(claws)), part('rope', 'trim', [.4, .2, 0], [rope, knot]), part('whippings', 'detail', [.5, .5, .3], [whipM].concat(binds))], { yaw: .35, fill: .92 });
}

export function balance(c) {
  const br = metal(c.body, { kind: 'cast', rough: .24 }), pan = metal(c.trim, { kind: 'cast', rough: .2, double: true }), st = metal(c.detail, { kind: 'cast', rough: .3 });
  const base = turned([[0, 0], [.42, 0], [.45, .03], [.42, .07], [.2, .1], [.1, .16], [.07, .24], [0, .24]], br, { sm: 50, seg: 40 });
  const post = turned([[.05, .22], [.035, .5], [.06, .62], [.03, .7], [.03, 1.5], [.055, 1.56], [.03, 1.62], [.02, 1.74], [0, 1.78]], br, { sm: 60, seg: 24 });
  // The beam tips a little: one pan carries more.
  const tilt = -.14, bp = x => V(x * Math.cos(tilt), 1.56 + x * Math.sin(tilt), 0);
  const beam = rod([bp(-.86), bp(-.5), bp(0).add(V(0, .05, 0)), bp(.5), bp(.86)], s => .022 + .016 * Math.sin(s * Math.PI), br, { n: 30, sides: 10 });
  const knobs = [-1, 1].map(k => { const p = bp(k * .88); return ball(.04, br, { p: [p.x, p.y, p.z] }); }), pointer = rod([[0, 1.56, .03], [-.03, 1.9, .03]], .012, br, { sides: 6 });
  const dish = k => { const top = bp(k * .82), py = top.y - .86; const prof = []; for (let i = 0; i <= 14; i++) { const r = i / 14 * .34; prof.push([r, .1 * Math.pow(r / .34, 2.4)]); } prof.push([.35, .11]);
    const d = turned(prof, pan, { sm: 0, seg: 48, p: [top.x, py, 0] }), lip = torus(.345, .012, pan, { r: [H2, 0, 0], p: [top.x, py + .105, 0] });
    const chains = [0, 1, 2].map(i => { const a = i / 3 * TAU + .5; return chain([[top.x, top.y - .06, 0], [top.x + Math.cos(a) * .17, top.y - .44, Math.sin(a) * .17], [top.x + Math.cos(a) * .34, py + .11, Math.sin(a) * .34]], .016, pan); });
    const hook = torus(.03, .009, pan, { p: [top.x, top.y - .035, 0] });
    return { pan: [d, lip, hook].concat(chains), at: V(top.x, py, 0) };
  };
  const L = dish(-1), Rt = dish(1);
  // Weights in one pan, coins in the other.
  const weights = [[-.08, .09, 0], [.1, .07, .06], [0, .05, -.12]].map(([x, s, z]) => turned([[0, 0], [s, 0], [s * 1.05, s * .3], [s * .8, s * 1.4], [s * .3, s * 1.5], [s * .34, s * 1.9], [0, s * 2]], st, { sm: 30, seg: 20, p: [L.at.x + x, L.at.y + .02, z] }));
  const coins = [[-.06, 0, .02, .2], [.08, .012, .06, -.3], [0, .02, -.08, .1], [.02, .04, .0, .5]].map(([x, y, z, tl]) => cyl(.075, .075, .016, st, { seg: 20, r: [tl * .3, 0, tl * .4], p: [Rt.at.x + x, Rt.at.y + .03 + y, z] }));
  return item('Balance scales', [part('stand and beam', 'body', [0, 0, 0], [base, post, beam, pointer].concat(knobs)), part('pans', 'trim', [0, -.2, .4], L.pan.concat(Rt.pan)), part('weights and coins', 'detail', [0, .2, .7], weights.concat(coins))], { stand: true, yaw: .3, el: 12, fill: .9 });
}

export function trap(c) {
  const iron = metal(c.body, { kind: 'forged', rough: .52 }), spring = metal(c.trim, { kind: 'forged', rough: .4 }), pl = metal(c.detail, { kind: 'forged', rough: .45 });
  // Set and waiting: the two jaws lie open, hinged at either end of the base bar.
  const A = .78, jaw = k => { const tip = .24, pts = []; for (let i = 0; i <= 40; i++) { const a = i / 40 * Math.PI, out = Math.sin(a) * .7; pts.push(V(-Math.cos(a) * A, .1 + out * Math.sin(tip), k * out * Math.cos(tip))); } return pts; };
  const jaws = [-1, 1].map(k => M(tube(jaw(k), .055, 6), iron));
  const teeth = [-1, 1].map(k => { const jp = jaw(k), tg = new THREE.ConeGeometry(.05, .26, 4, 1), at = []; for (let i = 4; i <= 36; i += 4) { const p = jp[i]; at.push({ p: [p.x, p.y + .14, p.z], r: [-k * .2, 0, 0] }); } return many(tg, iron, at); });
  const baseBar = rbox(A * 2 + .3, .05, .16, .02, iron, { p: [0, .03, 0] }), cross = rbox(.14, .04, 1.0, .015, iron, { p: [0, .025, 0] });
  const ears = [-1, 1].map(k => rbox(.05, .22, .2, .015, iron, { p: [k * (A + .04), .11, 0] }));
  // A leaf spring at each end, pressed down under the jaws.
  const springs = [-1, 1].map(k => bar([[k * (A + .1), .2], [k * (A + .44), .18], [k * (A + .62), .09], [k * (A + .44), .03], [k * (A + .08), .04]].map(p => V(p[0], p[1], 0)), .028, .15, spring)).flat();
  const eyes = [-1, 1].map(k => torus(.085, .028, spring, { r: [H2, 0, 0], p: [k * (A + .02), .2, 0] }));
  // The pan in the middle is what gets stepped on.
  const plate = turned([[0, .02], [.24, .02], [.26, 0], [.24, -.015], [0, -.015]], pl, { sm: 0, seg: 32, p: [0, .2, 0] }), stem = cyl(.025, .025, .16, pl, { p: [0, .11, 0] }), dog = rbox(.04, .03, .4, .01, pl, { p: [.0, .19, .34], r: [.15, 0, 0] });
  const ch = chain([[A + .7, .06, .1], [A + .9, .05, .4], [A + .6, .05, .7], [A + .2, .05, .86], [A - .2, .05, .8]], .045, pl), stake = torus(.1, .028, pl, { r: [H2, 0, 0], p: [A - .32, .05, .78] });
  return item('Bear trap', [part('jaws', 'body', [0, .3, 0], jaws.concat(teeth, [baseBar, cross], ears)), part('springs', 'trim', [0, 0, .4], springs.concat(eyes)), part('pan and chain', 'detail', [.3, .3, .5], [plate, stem, dog, ch, stake])], { stand: true, yaw: .4, el: 40, fill: .92 });
}

export function mirror(c) {
  const fr = metal(c.body, { kind: 'cast', rough: .24 }), silver = metal(c.trim, { rough: .04, bump: 0 }), st = gem(c.detail, { round: true, edges: 0, inner: .5 });
  silver.map = null; silver.roughnessMap = null;
  const oval = (r, n) => Array.from({ length: n }, (_, i) => { const a = i / n * TAU; return [Math.cos(a) * r * .82, .55 + Math.sin(a) * r]; });
  const back = slab(shape(oval(.74, 48), true), .07, .03, fr, { bseg: 4 });
  const face = slab(shape(oval(.6, 48), true), .02, .008, silver, { p: [0, 0, .04] });
  const bead = rod(oval(.66, 80).map(p => V(p[0], p[1], .045)), .035, fr, { closed: true, sides: 8 });
  const pearls = many(new THREE.SphereGeometry(.03, 10, 8), fr, oval(.74, 26).map(p => ({ p: [p[0], p[1], .03] })));
  // The handle: turned, with a knop where it meets the frame and a stone in its end.
  const handle = turned([[.07, -.2], [.1, -.26], [.06, -.34], [.05, -.6], [.065, -.9], [.05, -1.06], [.08, -1.12], [.085, -1.2], [.05, -1.26], [0, -1.27]], fr, { sm: 80, seg: 28, s: [1, 1, .8] });
  const leaf = [-1, 1].map(k => slab([[0, -.16], [k * .26, -.04], [k * .16, -.2], [k * .06, -.3]], .05, .02, fr, { curved: true, bseg: 3 }));
  const stone = ball(.06, st, { p: [0, -1.2, .07], s: [1, 1.2, .6] }), crest = ball(.07, st, { p: [0, 1.33, .03], s: [1, 1, .6] }), cb = torus(.075, .02, fr, { p: [0, 1.33, .02] });
  return item('Hand mirror', [part('frame and handle', 'body', [0, 0, 0], [back, bead, pearls, handle, cb].concat(leaf)), part('glass', 'trim', [0, .3, .5], [face]), part('stones', 'detail', [0, -.3, .6], [stone, crest])], { yaw: .4, fill: .94 });
}

export function chalice(c) {
  const gold = metal(c.body, { kind: 'cast', rough: .34, double: true }), sil = metal(c.trim, { kind: 'cast', rough: .44 }), st = gem(c.detail, { round: true, edges: 0, inner: .6 }), wine = liquid('#5a0f1e', { lit: .12 });
  const foot = turned([[0, 0], [.4, 0], [.43, .03], [.4, .06], [.22, .1], [.1, .2], [.06, .32]], gold, { sm: 50, seg: 48 });
  const stem = turned([[.06, .3], [.05, .44], [.06, .5]], gold, { sm: 12, seg: 24 }), knop = turned([[.06, .48], [.15, .54], [.17, .6], [.15, .66], [.06, .72]], sil, { sm: 30, seg: 32 }), stem2 = turned([[.06, .7], [.05, .82], [.09, .9]], gold, { sm: 12, seg: 24 });
  // The cup: a deep bowl with a thin lip, open at the top.
  const cup = turned(smooth([[.09, .88], [.2, .92], [.33, 1.06], [.4, 1.3], [.42, 1.56], [.44, 1.66]], 40).concat(smooth([[.425, 1.66], [.405, 1.56], [.385, 1.3], [.31, 1.08], [.18, .95], [0, .93]], 40)), gold, { sm: 0, seg: 56 });
  const drink = turned([[0, 1.5], [.4, 1.5], [.4, 1.48], [0, 1.48]], wine, { sm: 0, seg: 40 });
  const bandM = band(.405, 1.36, .12, .014, sil), lip = torus(.435, .012, sil, { r: [H2, 0, 0], p: [0, 1.66, 0] });
  const stones = Array.from({ length: 8 }, (_, i) => { const a = i / 8 * TAU + .2; return ball(.04, st, { p: [Math.cos(a) * .425, 1.36, Math.sin(a) * .425], s: [1, 1.2, 1] }); });
  const kstones = Array.from({ length: 6 }, (_, i) => { const a = i / 6 * TAU; return ball(.03, st, { p: [Math.cos(a) * .17, .6, Math.sin(a) * .17] }); });
  const lobes = many(new THREE.SphereGeometry(.05, 12, 8), gold, Array.from({ length: 12 }, (_, i) => { const a = i / 12 * TAU; return { p: [Math.cos(a) * .3, .07, Math.sin(a) * .3], s: [1, .5, 1.6], r: [0, -a, 0] }; }));
  return item('Chalice', [part('cup and foot', 'body', [0, 0, 0], [foot, stem, stem2, cup, lobes, drink]), part('knop and band', 'trim', [0, 0, .5], [knop, bandM, lip]), part('stones', 'detail', [0, .3, .8], stones.concat(kstones))], { stand: true, yaw: .3, el: 16, fill: .88 });
}

export function bell(c) {
  const bronze = metal(c.body, { kind: 'cast', rough: .3, double: true }), wd = wood(c.trim, { polish: .4 }), hide = leather(c.detail);
  // Standing on its lip; the clapper hangs just inside.
  const body = turned(smooth([[.07, 1.0], [.2, .98], [.27, .88], [.3, .7], [.34, .4], [.44, .16], [.56, .04], [.58, 0]], 50).concat(smooth([[.54, 0], [.42, .14], [.31, .4], [.27, .7], [.24, .86], [.18, .94], [0, .96]], 40)), bronze, { sm: 0, seg: 64 });
  const lines = [[.2, .43], [.26, .4], [.78, .292]].map(([y, R]) => torus(R, .012, bronze, { r: [H2, 0, 0], p: [0, y, 0] }));
  const crown = turned([[.07, .98], [.09, 1.02], [.06, 1.08], [.05, 1.12]], bronze, { sm: 16, seg: 24 });
  const handle = turned([[.05, 1.1], [.075, 1.16], [.06, 1.3], [.085, 1.5], [.07, 1.66], [.09, 1.72], [.07, 1.8], [0, 1.82]], wd, { sm: 60, seg: 28 });
  const ferr = band(.052, 1.12, .05, .012, bronze);
  const clapper = [rod([[0, .9, 0], [.04, .4, .02], [.08, .14, .03]], .014, bronze, { n: 10, sides: 6 }), ball(.075, bronze, { p: [.085, .08, .03], s: [1, 1.2, 1] })];
  const loop = rod([[0, 1.74, .08], [.14, 1.9, .1], [.02, 2.1, .1], [-.12, 1.9, .1], [0, 1.74, .08]], .016, hide, { n: 24, sides: 6 }), knot = torus(.085, .018, hide, { r: [H2, 0, 0], p: [0, 1.73, 0] });
  const g = [body, crown, ferr].concat(lines, clapper);
  return item('Hand bell', [part('bell', 'body', [0, 0, 0], g), part('handle', 'trim', [0, .5, 0], [handle]), part('thong', 'detail', [0, .8, .3], [loop, knot])], { stand: true, yaw: .3, el: 12, fill: .86 });
}
