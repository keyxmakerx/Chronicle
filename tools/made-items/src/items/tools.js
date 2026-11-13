// Tools of a trade.
import { THREE, V, M, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, slab, circle, blob, sheet, chain, many, aim, bar, drawn, flame, lampGlow, wrap, band, tube, path, loft, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, gem, bone, rock, paper, matte, glass, liquid, glow, dark } from '../mats.js';

export function pickaxe(c) {
  const iron = metal(c.body, { kind: 'forged', rough: .5 }), wd = wood(c.trim), hide = leather(c.detail);
  const haft = turned([[.055, -1.7], [.05, -.6], [.046, .5], [.058, .86]], wd, { sm: 30, seg: 28, s: [1, 1, .74] });
  const arc = []; for (let i = 0; i <= 40; i++) { const s = i / 40 * 2 - 1; arc.push(V(s * 1.05, .76 - .3 * s * s, 0)); }
  const head = bar(arc, s => .05 + .13 * (1 - Math.pow(Math.abs(s * 2 - 1), 1.3)), s => .03 + .09 * (1 - Math.pow(Math.abs(s * 2 - 1), 1.1)), iron);
  const eye = rbox(.2, .26, .17, .04, iron, { p: [0, .74, 0] });
  const grip = wrap(-.8, -1.6, () => .054, hide, { turns: 28, sz: .74 });
  return item('Pickaxe', [part('haft', 'trim', [0, -.2, 0], [haft]), part('head', 'body', [0, .4, 0], head.concat([eye])), part('grip', 'detail', [.3, -.3, 0], grip)], { yaw: -.55, fill: .95 });
}

export function tongs(c, t) {
  const iron = metal(c.body, { kind: 'forged', rough: .55 }), hide = leather(c.trim), ph = (t || 0) * Math.PI * 2, heat = 1 + .3 * Math.sin(ph) + .08 * Math.sin(ph * 5);
  const arm = k => { const pts = [[k * .2, -1.7, 0], [k * .15, -1.0, 0], [k * .05, -.3, 0], [0, 0, 0], [-k * .1, .25, 0], [-k * .13, .55, 0], [-k * .06, .82, 0], [-k * .035, .98, 0]]; return rod(pts, s => s < .62 ? .035 : .042 - (s - .62) * .03, iron, { n: 50, sides: 10, s: [1, 1, .9], p: [0, 0, k * .03] }); };
  const pin = turned([[0, .05], [.05, .045], [.06, .02], [.06, 0]], iron, { sm: 12, r: [Math.PI / 2, 0, 0], p: [0, 0, .06] });
  const wraps = [-1, 1].map(k => wrap(-1.0, -1.66, () => .037, hide, { turns: 18, sz: .9 })); wraps.forEach((w, i) => w.forEach(m => { const k = i ? 1 : -1; m.geometry.rotateZ(k * .075); m.geometry.translate(k * .12 + k * .0, 0, k * .03); }));
  const hot = rbox(.13, .5, .1, .02, glow(c.detail, 2.2 * heat), { p: [0, 1.12, 0], r: [0, 0, .1] }), hotTip = rbox(.13, .22, .1, .02, glow('#ffd9a0', 3 * heat), { p: [-.03, 1.3, 0], r: [0, 0, .1] });
  return item('Forge tongs', [part('arms', 'body', [0, 0, 0], [arm(1), arm(-1)]), part('pin', 'trim', [0, 0, .4], [pin]), part('wraps', 'trim', [0, -.4, 0], wraps.flat()), part('hot work', 'detail', [0, .5, 0], [hot, hotTip])], { yaw: -.6, fill: .95, lights: [{ p: [0, 1.15, .25], color: c.detail, power: 1.6 * heat }], halo: .9 });
}

export function mortar(c) {
  const st = rock(c.body, { bump: 1.2, rough: .7 }), wd = wood(c.trim, { polish: .3 }), herb = matte(c.detail, { rough: .9, bump: 1 }), n = makeNoise(8, 16), r = rng(5);
  const bowl = turned([[0, 0], [.4, 0], [.44, .04], [.4, .1], [.36, .16], [.5, .3], [.66, .6], [.7, .88], [.69, .95], [.63, .96], [.6, .9], [.55, .6], [.4, .36], [.2, .28], [0, .26]], st, { sm: 120, seg: 72 });
  const mash = blob([.5, .1, .5], v => 1 + (n(v.x * 4 + 2, v.z * 4) - .5) * .5, herb, { p: [0, .42, 0] });
  const bits = many(new THREE.IcosahedronGeometry(.035, 0), herb, Array.from({ length: 40 }, () => { const a = r() * 6.28, d = Math.sqrt(r()) * .42; return { p: [Math.cos(a) * d, .5 + r() * .04, Math.sin(a) * d], r: [r() * 3, r() * 3, 0], s: .6 + r() }; }));
  const pestle = turned([[0, 0], [.1, .01], [.13, .1], [.1, .3], [.065, .7], [.06, 1.1], [.07, 1.16], [.05, 1.22], [0, 1.23]], wd, { sm: 60, r: [0, 0, -.62], p: [.02, .44, .06] });
  return item('Mortar and pestle', [part('bowl', 'body', [0, -.2, 0], [bowl]), part('what is being ground', 'detail', [0, .5, 0], [mash, bits]), part('pestle', 'trim', [.5, .5, 0], [pestle])], { stand: true, yaw: .3, el: 26 });
}

export function lantern(c, t) {
  const iron = metal(c.body, { kind: 'forged', rough: .5 }), br = metal(c.trim, { kind: 'cast' }), gl = glass('#e8e2c8', { tint: .1 });
  const base = turned([[0, 0], [.44, 0], [.47, .03], [.45, .08], [.36, .11], [.33, .2], [.36, .24], [0, .24]], br, { sm: 60 });
  const font = turned([[0, .24], [.2, .24], [.22, .34], [.12, .42], [.05, .46], [0, .46]], br, { sm: 30 });
  const pane = turned([[.31, .25], [.34, .6], [.34, .95], [.3, 1.22]], gl, { sm: 30, seg: 48 });
  const posts = []; for (let i = 0; i < 4; i++) { const a = i / 4 * Math.PI * 2 + Math.PI / 4; posts.push(rod([[Math.cos(a) * .34, .22, Math.sin(a) * .34], [Math.cos(a) * .375, .6, Math.sin(a) * .375], [Math.cos(a) * .375, .95, Math.sin(a) * .375], [Math.cos(a) * .32, 1.24, Math.sin(a) * .32]], .02, iron, { n: 20, sides: 8 })); }
  const hoops = [.6, .95].map(y => torus(.375, .014, iron, { r: [Math.PI / 2, 0, 0], p: [0, y, 0] }));
  const cap = turned([[.36, 1.22], [.38, 1.25], [.3, 1.34], [.16, 1.44], [.14, 1.5], [.2, 1.53], [.19, 1.57], [.08, 1.62], [0, 1.62]], iron, { sm: 60 });
  const bail = rod([[-.37, 1.26, 0], [-.5, 1.6, 0], [-.3, 1.98, 0], [0, 2.08, 0], [.3, 1.98, 0], [.5, 1.6, 0], [.37, 1.26, 0]], .018, br, { n: 50, sides: 8 });
  const wick = cyl(.012, .012, .08, dark(), { p: [0, .5, 0] });
  const fl = flame(.34, .07, c.detail, t, () => glow('#ffffff', 1, { vc: true, add: true })); fl.forEach(m => { m.geometry.translate(0, .52, 0); m.renderOrder = 1; });
  return item('Lantern', [part('base', 'trim', [0, -.35, 0], [base, font]), part('frame', 'body', [0, 0, 0], posts.concat(hoops)), part('glass', 'detail', [0, 0, .8], [pane]), part('flame', 'detail', [0, .1, 0], [wick].concat(fl)), part('cap', 'body', [0, .4, 0], [cap]), part('handle', 'trim', [0, .7, 0], [bail])], { stand: true, yaw: .3, el: 10, lights: [{ p: [0, .72, 0], color: c.detail, power: 3.2, flick: .2 }], halo: 1.1 });
}

export function key(c) {
  const brz = metal(c.body, { kind: 'cast', rough: .4 }), gold = metal(c.trim, { kind: 'cast' }), st = gem(c.detail);
  const stem = turned([[.045, -1.25], [.05, -1.2], [.04, -1.1], [.04, .2], [.055, .26], [.04, .32]], brz, { sm: 0, seg: 24 });
  const tip = ball(.05, brz, { p: [0, -1.27, 0] });
  const bitS = slab([[.03, -1.15], [.36, -1.15], [.36, -1.02], [.24, -1.02], [.24, -.94], [.36, -.94], [.36, -.8], [.2, -.8], [.2, -.72], [.36, -.72], [.36, -.6], [.03, -.6]], .07, .012, brz);
  const loop = torus(.3, .055, brz, { p: [0, .62, 0], seg: 64 }), inner = []; for (let i = 0; i < 4; i++) { const a = i / 4 * Math.PI * 2 + Math.PI / 4; inner.push(torus(.105, .03, brz, { p: [Math.cos(a) * .17, .62 + Math.sin(a) * .17, 0] })); }
  const collar = [band(.04, .2, .1, .02, gold), band(.04, -.5, .06, .014, gold)];
  const bez = turned([[0, .05], [.07, .05], [.09, .03], [.095, 0]], gold, { sm: 12, r: [Math.PI / 2, 0, 0], p: [0, .62, .02] }), stone = ball(.06, st, { s: [1, 1, .7], p: [0, .62, .07] });
  return item('Key', [part('stem and bit', 'body', [0, -.3, 0], [stem, tip, bitS]), part('bow', 'body', [0, .3, 0], [loop].concat(inner)), part('collar', 'trim', [.4, 0, 0], collar), part('stone', 'detail', [0, .3, .6], [bez, stone])], { yaw: .7, fill: .95 });
}

export function spool(c) {
  const wd = wood(c.trim, { polish: .3 }), thr = cloth(c.body, { sheen: 1, bump: .1 }), steel = metal(c.detail, { rough: .15 });
  const core = turned([[0, 0], [.42, 0], [.45, .03], [.44, .1], [.2, .13], [.19, .2], [.19, .96], [.2, 1.03], [.44, 1.06], [.45, 1.13], [.42, 1.16], [0, 1.16]], wd, { sm: 0, seg: 56 });
  const wp = []; for (let i = 0; i <= 120; i++) { const s = i / 120, bulge = Math.sin(s * Math.PI); wp.push([.3 + .09 * Math.pow(bulge, .5) + .006 * Math.sin(s * 150), .13 + s * .9]); }
  const wound = M(lathe(wp, 64), thr);
  const loose = rod([[.39, .5, .0], [.5, .3, .2], [.75, .02, .35], [1.05, .015, .1], [1.2, .015, -.3], [.95, .015, -.5]], .012, thr, { n: 60, sides: 6 });
  const needle = turned([[0, 0], [.012, .1], [.016, .9], [.02, 1.02], [.012, 1.12], [0, 1.14]], steel, { sm: 30, seg: 12, r: [0, 0, -.9], p: [-.36, .28, .3] });
  return item('Needle and thread', [part('spool', 'trim', [0, -.3, 0], [core]), part('thread', 'body', [0, .1, 0], [wound, loose]), part('needle', 'detail', [0, .5, .5], [needle])], { stand: true, yaw: .4, el: 24 });
}

export function anvil(c) {
  const steel = metal(c.body, { kind: 'forged', rough: .42, bump: .35 }), wd = wood(c.trim, { bump: 1.4 }), iron = metal(c.detail === '#2c2622' ? '#4a4d52' : c.detail, { kind: 'forged' });
  const stump = turned([[0, 0], [.6, 0], [.62, .05], [.58, .7], [.56, .74], [0, .74]], wd, { sm: 0, seg: 48 });
  const hoop = band(.585, .56, .09, .016, iron), hoop2 = band(.6, .16, .09, .016, iron);
  // The anvil: feet, a waist, the body with its flat face, the horn and the heel.
  const sec = (x, w, y0, y1) => [V(x, y0, w), V(x, y1, w), V(x, y1, -w), V(x, y0, -w)];
  const feet = M(loft([sec(-.5, .26, .74, .78), sec(-.46, .24, .74, .9), sec(-.2, .12, .74, 1.0), sec(.2, .12, .74, 1.0), sec(.46, .24, .74, .9), sec(.5, .26, .74, .78)], { sharp: true }), steel);
  const body = rbox(1.0, .26, .34, .03, steel, { p: [.06, 1.13, 0] }), waist = rbox(.5, .2, .26, .04, steel, { p: [0, .96, 0] });
  const horn = M(new THREE.ConeGeometry(.14, .62, 24), steel, { r: [0, 0, Math.PI / 2], s: [1, .86, 1.1], p: [-.75, 1.14, 0] });
  const face = rbox(.98, .03, .33, .01, metal('#b8bec6', { rough: .2 }), { p: [.07, 1.268, 0] });
  const hardy = rbox(.06, .01, .06, .002, dark(), { p: [.42, 1.286, 0] });
  return item('Anvil', [part('block', 'trim', [0, -.4, 0], [stump]), part('band', 'trim', [.6, -.3, 0], [hoop, hoop2]), part('anvil', 'body', [0, .4, 0], [feet, body, waist, horn, face, hardy])], { stand: true, yaw: .5, el: 16 });
}

export function compass(c) {
  const br = metal(c.body, { kind: 'cast', rough: .25 }), red = metal(c.detail, { rough: .3 }), steel = metal('#c8ccd2', { rough: .2 }), gl = glass('#dfe8e8', { tint: .05 });
  const card = drawn(512, 512, (x, w, h) => { x.translate(w / 2, h / 2); x.strokeStyle = '#3a2a1a'; x.fillStyle = '#3a2a1a'; x.lineWidth = 3; x.beginPath(); x.arc(0, 0, 236, 0, 7); x.stroke(); x.lineWidth = 1.5; x.beginPath(); x.arc(0, 0, 200, 0, 7); x.stroke();
    for (let i = 0; i < 64; i++) { const a = i / 64 * Math.PI * 2, l = i % 8 === 0 ? 30 : i % 4 === 0 ? 20 : 10; x.beginPath(); x.moveTo(Math.cos(a) * 236, Math.sin(a) * 236); x.lineTo(Math.cos(a) * (236 - l), Math.sin(a) * (236 - l)); x.stroke(); }
    for (let i = 0; i < 8; i++) { const a = i / 8 * Math.PI * 2, L = i % 2 ? 110 : 190; x.save(); x.rotate(a); x.beginPath(); x.moveTo(0, -L); x.lineTo(16, -20); x.lineTo(0, 0); x.closePath(); x.fill(); x.beginPath(); x.moveTo(0, -L); x.lineTo(-16, -20); x.lineTo(0, 0); x.closePath(); x.stroke(); x.restore(); }
    x.fillStyle = '#7a1f1f'; x.font = 'bold 54px serif'; x.textAlign = 'center'; x.fillText('N', 0, -196); });
  const caseM = turned([[0, 0], [.6, 0], [.64, .03], [.65, .14], [.62, .18], [.57, .17], [.56, .07], [0, .06]], br, { sm: 0, seg: 72, r: [Math.PI / 2, 0, 0] });
  const face = M(new THREE.CircleGeometry(.56, 64), paper('#ffffff', { map: card }), { p: [0, 0, .071] });
  const needle = [slab([[0, .44], [.05, 0], [-.05, 0]], .016, .006, red, { p: [0, 0, .12], r: [0, 0, .5] }), slab([[0, -.44], [.05, 0], [-.05, 0]], .016, .006, steel, { p: [0, 0, .12], r: [0, 0, .5] })];
  const pin = ball(.035, br, { p: [0, 0, .13] });
  const pane = M(new THREE.SphereGeometry(1.2, 48, 12, 0, Math.PI * 2, 0, .49), gl, { r: [Math.PI / 2, 0, 0], p: [0, 0, -.9] });
  const stemM = turned([[.07, .62], [.08, .7], [.06, .76]], br, { sm: 12 }), loop = torus(.1, .025, br, { p: [0, .84, .09], r: [.4, 0, 0] });
  stemM.geometry.translate(0, 0, .09);
  return item('Compass', [part('case', 'body', [0, 0, -.2], [caseM, stemM]), part('card', '', [0, 0, .2], [face]), part('needle', 'detail', [0, 0, .6], needle.concat([pin])), part('glass', '', [0, 0, 1], [pane]), part('loop', 'trim', [0, .4, 0], [loop])], { yaw: .25, fill: .9 });
}
