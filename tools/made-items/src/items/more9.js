// Magic and the table: a holy symbol, prayer beads, an idol, a genie lamp, a deck of cards and an astrolabe; and apples, grapes, mushrooms, a teapot and a keg.
import { THREE, V, M, part, item, turned, smooth, lathe, rod, ball, torus, cyl, rbox, slab, shape, hole, circle, blob, sheet, many, aim, band, bar, fan, chain, cutStone, strands, rng, drawn } from '../kit.js';
import { metal, wood, cloth, matte, rock, paper, gem, liquid, glow, ghost, dark } from '../mats.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
// A profile turned so its surface faces out, as the lathe does for whole turns.
const outward = P => { let s = 0; for (let i = 0; i < P.length - 1; i++) s += (P[i + 1][1] - P[i][1]) * (P[i][0] + P[i + 1][0]); return s < 0 ? P.slice().reverse() : P; };
// The radius of a turned profile at a height, for things that sit on its surface.
const radiusAt = P => y => { for (let i = 0; i < P.length - 1; i++) { const a = P[i], b = P[i + 1]; if ((y - a[1]) * (y - b[1]) <= 0 && a[1] !== b[1]) return a[0] + (b[0] - a[0]) * (y - a[1]) / (b[1] - a[1]); } return P[P.length - 1][0]; };

export function holysymbol(c, t) {
  const ph = (t || 0) * TAU, gold = metal(c.body, { kind: 'cast', rough: .26 }), links = metal(c.trim, { kind: 'cast', rough: .3 }), stone = gem(c.detail, { round: true, edges: 0, inner: 1.3 + .5 * Math.sin(ph), lit: .3 + .15 * Math.sin(ph) });
  // A sun: eight long rays and eight short ones round a boss that holds the stone.
  const pts = []; for (let i = 0; i < 32; i++) { const a = i / 32 * TAU + H2, r = i % 4 === 0 ? 1 : i % 4 === 2 ? .74 : .5; pts.push([Math.cos(a) * r, Math.sin(a) * r]); }
  const sun = slab(pts, .07, .03, gold, { bseg: 2 });
  const boss = turned([[0, .07], [.28, .07], [.38, .045], [.46, 0]], gold, { sm: 30, r: [H2, 0, 0], p: [0, 0, .03] });
  const bezel = torus(.25, .028, gold, { p: [0, 0, .1] }), stoneM = ball(.235, stone, { s: [1, 1, .5], p: [0, 0, .1] });
  const ridges = []; for (let i = 0; i < 8; i++) { const a = i / 8 * TAU + H2, x = Math.cos(a), y = Math.sin(a); ridges.push(rod([[x * .46, y * .46, .045], [x * .96, y * .96, .01]], s => .028 * (1 - s) + .004, gold, { sides: 8 })); }
  const bail = torus(.1, .028, gold, { r: [0, H2, 0], p: [0, 1.07, 0] });
  const loop = []; for (let i = 0; i <= 24; i++) { const a = -H2 + i / 24 * TAU; loop.push(V(Math.cos(a) * .46, 1.72 + Math.sin(a) * .62, .04 * Math.sin(a * 2))); }
  const chainM = chain(loop, .04, links);
  // The light it gives: faint rays behind the sun that turn slowly.
  const rp = []; for (let i = 0; i < 16; i++) { const a = i / 16 * TAU + (t || 0) * TAU / 8, r = i % 2 ? .3 : 1.5; rp.push([Math.cos(a) * r, Math.sin(a) * r]); }
  const rays = M(new THREE.ShapeGeometry(shape(rp)), ghost(c.detail, .07 + .03 * Math.sin(ph)), { p: [0, 0, -.1], nofit: true });
  return item('Holy symbol', [part('sun', 'body', [0, 0, 0], [sun, boss, bezel, bail].concat(ridges)), part('stone', 'detail', [0, 0, .8], [stoneM, rays]), part('chain', 'trim', [0, .5, 0], [chainM])], { fill: .95, lights: [{ p: [0, 0, .5], color: c.detail, power: .5 + .2 * Math.sin(ph) }], halo: .8 });
}

export function beads(c) {
  const wd = wood(c.body, { polish: .7, bump: .3 }), br = metal(c.trim, { kind: 'cast', rough: .28 }), silk = cloth(c.detail, { sheen: 1 }), cordM = matte('#3a2a20', { rough: .9 }), r = rng(5);
  // One loop of beads with a counting bead of metal after every eighth, and a tassel under the largest.
  const P = a => V(Math.sin(a) * .8 * (1 + .1 * Math.cos(a)), .42 - Math.cos(a) * 1.0, .05 * Math.sin(a * 3)), N = 32, plain = [], count = [];
  for (let i = 0; i < N; i++) { const p = P((i + 1) / (N + 1) * TAU); if (i % 8 === 7) count.push({ p: p, s: 1.1 }); else plain.push({ p: p, s: .92 + r() * .16 }); }
  const beadsM = many(new THREE.SphereGeometry(.08, 20, 14), wd, plain), counters = many(new THREE.SphereGeometry(.08, 20, 14), br, count);
  const cp = []; for (let i = 0; i < 64; i++) cp.push(P(i / 64 * TAU));
  const cord = rod(cp, .012, cordM, { sides: 6, closed: true });
  const big = ball(.14, wd, { p: [0, -.6, 0] }), small = ball(.07, wd, { p: [0, -.8, 0], s: [1, .8, 1] }), collar = band(.05, -.72, .04, .02, br);
  const head = turned([[0, -.84], [.05, -.85], [.09, -.92], [.085, -1.0], [.06, -1.05]], silk, { sm: 20 }), tie = band(.062, -1.04, .04, .014, br);
  const tassel = strands(46, rr => V((rr() - .5) * .08, -1.05, (rr() - .5) * .08), (rr, i, a) => V(a.x * 2.2 + (rr() - .5) * .08, -1.74 - rr() * .08, a.z * 2.2 + (rr() - .5) * .08), .011, silk, 9);
  return item('Prayer beads', [part('beads', 'body', [0, .3, 0], [beadsM, big, small]), part('counting beads', 'trim', [0, 0, .6], [counters, collar, tie]), part('tassel', 'detail', [0, -.6, 0], [head].concat(tassel)), part('cord', '', [0, 0, -.4], [cord])], { fill: .95 });
}

export function idol(c) {
  const st = rock(c.body, { rough: .8, bump: 1.3 }), gold = metal(c.trim, { kind: 'cast', rough: .3 }), eye = gem(c.detail, { inner: 1.8, lit: .35 }), sq = [1, 1, .86];
  // A squat seated figure cut from one stone: the head as big as the body, the hands folded on the belly.
  const plinth = rbox(1.36, .18, 1.1, .03, st, { p: [0, .09, .04] });
  const body = turned([[0, .18], [.52, .18], [.58, .3], [.54, .7], [.4, .92], [0, .94]], st, { sm: 40, s: sq });
  const head = turned([[0, .9], [.36, .9], [.47, 1.0], [.5, 1.45], [.46, 1.66], [.3, 1.78], [0, 1.8]], st, { sm: 40, s: sq });
  const knot = turned([[0, 1.76], [.2, 1.76], [.25, 1.86], [.16, 1.98], [0, 2.0]], st, { sm: 20 });
  const brow = rbox(.74, .1, .16, .04, st, { p: [0, 1.52, .38] }), nose = rbox(.15, .32, .16, .05, st, { p: [0, 1.34, .42] });
  const lips = [rbox(.4, .05, .1, .02, st, { p: [0, 1.15, .4] }), rbox(.4, .05, .1, .02, st, { p: [0, 1.06, .4] })], mouth = rbox(.34, .06, .06, .02, dark(), { p: [0, 1.105, .4] });
  const eyes = [-1, 1].map(k => M(cutStone(.065, .03, .05), eye, { p: [k * .21, 1.39, .44] })), rims = [-1, 1].map(k => torus(.08, .026, st, { s: [1.25, 1, 1], p: [k * .21, 1.39, .425] }));
  const ears = [-1, 1].map(k => blob([.07, .26, .13], null, st, { p: [k * .5, 1.3, 0] })), spools = [-1, 1].map(k => ball(.05, gold, { p: [k * .53, 1.08, .02] }));
  const arms = [-1, 1].map(k => rod([[k * .5, .8, .05], [k * .64, .52, .26], [k * .4, .42, .5], [k * .1, .44, .52]], .095, st, { n: 24, sides: 12 }));
  const legs = [-1, 1].map(k => blob([.38, .13, .2], null, st, { r: [0, k * -.3, 0], p: [k * .26, .3, .46] }));
  const circlet = band(.455, 1.68, .06, .03, gold, { s: sq }), collar = band(.42, .93, .1, .04, gold, { s: sq }), cuffs = [-1, 1].map(k => torus(.105, .028, gold, { r: [0, H2, 0], p: [k * .3, .425, .51] }));
  return item('Idol', [part('figure', 'body', [0, 0, 0], [plinth, body, head, knot, brow, nose, mouth].concat(lips, rims, ears, arms, legs)), part('gold', 'trim', [0, .6, .4], [circlet, collar].concat(spools, cuffs)), part('eyes', 'detail', [0, .3, .9], eyes)], { stand: true, yaw: .3, el: 12, lights: [{ p: [0, 1.39, .9], color: c.detail, power: .25 }] });
}

export function genielamp(c, t) {
  const ph = (t || 0) * TAU, br = metal(c.body, { kind: 'cast', rough: .24 }), dk = metal(c.trim, { kind: 'cast', rough: .34 }), stone = gem(c.detail, { round: true, edges: 0, inner: 1.2 });
  const foot = turned([[0, 0], [.34, 0], [.36, .04], [.24, .1], [.16, .2], [.2, .3]], dk, { sm: 30 });
  const body = blob([.7, .3, .46], v => 1 + .1 * Math.max(0, -v.y), br, { p: [0, .56, 0] });
  const girdle = torus(.705, .026, dk, { s: [1, .657, 1], r: [H2, 0, 0], p: [0, .56, 0] });
  const spout = rod([[.4, .52, 0], [.8, .56, 0], [1.1, .74, 0], [1.32, 1.0, 0], [1.4, 1.12, 0]], s => .2 - .13 * s, br, { n: 30, sides: 16 });
  const lip = torus(.075, .022, dk, { r: [0, H2, .98], p: [1.4, 1.12, 0] }), mouth = ball(.06, dark(), { p: [1.4, 1.12, 0] }), ember = ball(.034, glow(c.detail, 2 + .6 * Math.sin(ph)), { p: [1.42, 1.15, 0] });
  const handle = rod([[-.55, .7, 0], [-.95, .98, 0], [-1.2, .8, 0], [-1.1, .48, 0], [-.8, .36, 0], [-.6, .42, 0]], .045, dk, { n: 40 }), rest = ball(.07, dk, { p: [-.95, 1.0, 0], s: [1.3, .7, 1] });
  const lid = turned([[.3, .8], [.3, .84], [.2, .94], [.08, 1.0], [.05, 1.05], [.08, 1.1], [.05, 1.16], [0, 1.18]], br, { sm: 40 }), lidRing = band(.3, .82, .05, .02, dk), stoneM = ball(.065, stone, { p: [0, 1.22, 0] });
  // Smoke climbs from the wick and curls, as if something were about to come out.
  const smoke = [0, 1, 2].map(i => { const pts = []; for (let j = 0; j <= 24; j++) { const s = j / 24; pts.push(V(1.42 + .12 * s + Math.sin(s * 5 + i * 2.1 - ph) * .12 * s, 1.16 + s * .95, Math.cos(s * 4 + i * 1.3 - ph) * .1 * s)); } return rod(pts, s => .012 + .075 * s, ghost(c.detail, .07), { sides: 8, nofit: true }); });
  return item('Genie lamp', [part('lamp', 'body', [0, 0, 0], [body, spout, lid]), part('fittings', 'trim', [-.4, 0, .3], [foot, girdle, lip, mouth, handle, rest, lidRing]), part('stone and smoke', 'detail', [.3, .6, .3], [stoneM, ember].concat(smoke))], { stand: true, yaw: .25, el: 14, lights: [{ p: [1.45, 1.2, 0], color: c.detail, power: .5, flick: .2 }], halo: .7 });
}

export function cards(c) {
  // The backs are drawn pale on grey so the recipe's colour comes through.
  const back = drawn(256, 368, (x, w, h) => {
    x.strokeStyle = '#ffffff'; x.lineWidth = 9; x.strokeRect(16, 16, w - 32, h - 32); x.lineWidth = 2; x.strokeRect(30, 30, w - 60, h - 60);
    x.save(); x.beginPath(); x.rect(34, 34, w - 68, h - 68); x.clip(); x.globalAlpha = .6; x.lineWidth = 3;
    for (let i = -h; i < w + h; i += 26) { x.beginPath(); x.moveTo(i, 0); x.lineTo(i + h, h); x.stroke(); x.beginPath(); x.moveTo(i, h); x.lineTo(i + h, 0); x.stroke(); }
    x.restore(); x.fillStyle = '#8c8c8c'; x.beginPath(); x.arc(w / 2, h / 2, 46, 0, TAU); x.fill(); x.lineWidth = 5; x.stroke(); x.fillStyle = '#ffffff'; x.beginPath(); x.arc(w / 2, h / 2, 18, 0, TAU); x.fill();
  }, [.55, .55, .55]);
  const face = drawn(256, 368, (x, w, h) => { x.strokeStyle = '#3a2a1c'; x.lineWidth = 3; x.strokeRect(18, 18, w - 36, h - 36); x.lineWidth = 1; x.strokeRect(26, 26, w - 52, h - 52); }, [.95, .92, .82]);
  const backM = paper(c.body, { map: back }), faceM = paper('#ffffff', { map: face }), gilt = metal(c.trim, { kind: 'cast', rough: .42 }), ink = matte(c.detail, { rough: .5 }), silk = cloth(c.detail, { sheen: 1 }), waxM = matte(c.detail, { rough: .35, wet: .5 });
  const W = .9, D = 1.3, flat = (mat, y, ry, px, pz) => sheet((u, v) => V((u - .5) * W, 0, (v - .5) * D), 2, 2, mat, { r: [0, ry, 0], p: [px, y, pz] });
  // The deck: gilt on its edges, tied with a ribbon under a wax seal, the top card face down.
  const stack = rbox(W, .3, D, .015, gilt, { p: [0, .15, 0] }), top = flat(backM, .302, 0, 0, 0);
  const loose = [rbox(W, .014, D, .004, gilt, { r: [0, .09, 0], p: [.03, .1, .02] }), rbox(W, .014, D, .004, gilt, { r: [0, -.07, 0], p: [-.02, .2, -.02] })];
  const ribbon = bar([[-.462, 0], [-.462, .28], [-.44, .314], [.44, .314], [.462, .28], [.462, 0]], .008, .16, silk);
  const seal = [blob([.13, .03, .13], v => 1 + .08 * Math.sin(Math.atan2(v.z, v.x) * 5), waxM, { p: [0, .33, 0] }), torus(.07, .012, waxM, { r: [H2, 0, 0], p: [0, .356, 0] })];
  // Three cards drawn and laid face up beside it.
  const lay = [[.86, .012, -.5, .34], [.98, .024, -.12, .52], [1.02, .036, .26, .7]], faces = [], pips = [];
  const pip = (pts, k) => slab(pts, .004, 0, ink, { r: [-H2, lay[k][2], 0], p: [lay[k][0], lay[k][1] + .003, lay[k][3]] });
  const diamond = (x, y, s) => [[x, y + s], [x + s * .7, y], [x, y - s], [x - s * .7, y]], star = (x, y, s) => { const q = []; for (let i = 0; i < 16; i++) { const a = i / 16 * TAU, r = i % 2 ? s * .45 : s; q.push([x + Math.cos(a) * r, y + Math.sin(a) * r]); } return q; };
  lay.forEach((l, k) => { faces.push(flat(faceM, l[1], l[2], l[0], l[3])); for (const [sx, sy] of [[-1, 1], [1, -1]]) pips.push(pip(diamond(sx * .33, sy * .52, .07), k)); });
  pips.push(pip(diamond(0, 0, .26), 0), pip(star(0, 0, .26), 1), pip(diamond(0, .3, .15), 2), pip(diamond(0, 0, .15), 2), pip(diamond(0, -.3, .15), 2));
  return item('Deck of cards', [part('gilt edges', 'trim', [0, 0, 0], [stack].concat(loose)), part('backs', 'body', [0, .5, 0], [top]), part('ribbon, seal and pips', 'detail', [0, .3, .5], ribbon.concat(seal, pips)), part('faces', '', [.6, 0, .3], faces)], { stand: true, yaw: .3, el: 40, fill: .95 });
}

export function astrolabe(c) {
  const br = metal(c.body, { kind: 'cast', rough: .3 }), plate = metal(c.body, { kind: 'cast', rough: .4 }), sv = metal(c.trim, { rough: .28 }), enamel = matte(c.detail, { rough: .25, wet: .8 }), stone = gem(c.detail, { round: true, edges: 0, inner: 1 });
  // The plate is engraved with the degrees, the tropics and the hour lines.
  plate.map = drawn(512, 512, (x, w) => {
    const m = w / 2; x.strokeStyle = 'rgba(40,25,10,.85)'; x.lineWidth = 2;
    for (const r of [244, 214, 150, 96]) { x.beginPath(); x.arc(m, m, r, 0, TAU); x.stroke(); }
    for (let i = 0; i < 72; i++) { const a = i / 72 * TAU, r0 = i % 6 ? 228 : 214; x.beginPath(); x.moveTo(m + Math.cos(a) * r0, m + Math.sin(a) * r0); x.lineTo(m + Math.cos(a) * 244, m + Math.sin(a) * 244); x.stroke(); }
    x.lineWidth = 1.5; for (let i = 1; i < 6; i++) { x.beginPath(); x.arc(m, m + 330 - i * 20, 300 - i * 32, Math.PI * 1.12, Math.PI * 1.88); x.stroke(); }
    x.beginPath(); x.moveTo(m - 214, m); x.lineTo(m + 214, m); x.moveTo(m, m - 214); x.lineTo(m, m + 214); x.stroke();
  }, [.82, .82, .82]);
  const mater = cyl(1, 1, .07, br, { seg: 64, r: [H2, 0, 0] }), faceM = M(new THREE.CircleGeometry(.9, 64), plate, { p: [0, 0, .037] }), limb = torus(.95, .05, br, { seg: 64, p: [0, 0, .03] }), inlay = torus(.87, .014, enamel, { seg: 64, p: [0, 0, .045] });
  const throne = slab(hole(shape([[-.32, .9], [-.22, 1.04], [-.14, 1.16], [-.07, 1.24], [0, 1.3], [.07, 1.24], [.14, 1.16], [.22, 1.04], [.32, .9]]), circle(0, 1.16, .05, 20)), .06, .012, br), shackle = torus(.08, .022, br, { r: [0, H2, 0], p: [0, 1.22, 0] }), hang = torus(.2, .03, br, { p: [0, 1.48, 0] });
  // The star net: a ring, the off-centre circle of the sun's road, and a pointer for each named star.
  const z = .062, net = [slab(hole(shape(circle(0, 0, .82, 56)), circle(0, 0, .75, 56)), .024, .006, sv, { p: [0, 0, z] }), slab(hole(shape(circle(0, .26, .47, 48)), circle(0, .26, .41, 48)), .024, .006, sv, { p: [0, 0, z] }), rbox(1.54, .05, .022, .006, sv, { p: [0, -.22, z] }), rbox(.05, .56, .022, .006, sv, { p: [0, -.5, z] })];
  for (const [x, y, a] of [[-.5, .5, 2.2], [.52, .46, .9], [-.62, -.02, 3.4], [.6, -.05, -.3], [-.3, -.5, 4.2], [.34, -.48, 5.2], [0, .52, 1.57]]) net.push(slab([[0, .03], [.2, 0], [0, -.03], [-.03, 0]], .022, .005, sv, { r: [0, 0, a], p: [x, y, z] }));
  // The rule turns on the pin, with a sight at each end.
  const ra = .62, rule = slab([[-.92, 0], [-.8, .05], [-.1, .06], [0, .09], [.1, .06], [.8, .05], [.92, 0], [.8, -.05], [.1, -.06], [0, -.09], [-.1, -.06], [-.8, -.05]], .02, .005, br, { r: [0, 0, ra], p: [0, 0, .095] });
  const sights = [-1, 1].map(k => rbox(.02, .1, .12, .004, br, { r: [0, 0, ra], p: [Math.cos(ra) * .7 * k, Math.sin(ra) * .7 * k, .15] }));
  const pin = turned([[0, .05], [.05, .05], [.07, .02], [.07, 0]], br, { sm: 0, seg: 24, r: [H2, 0, 0], p: [0, 0, .1] }), stoneM = ball(.05, stone, { s: [1, 1, .6], p: [0, 0, .16] });
  return item('Astrolabe', [part('plate and rule', 'body', [0, 0, -.3], [mater, faceM, limb, throne, shackle, hang, rule, pin].concat(sights)), part('star net', 'trim', [0, 0, .6], net), part('enamel and stone', 'detail', [0, 0, 1.1], [inlay, stoneM])], { fill: .95 });
}

const appleP = [[0, .84], [.07, .88], [.2, .97], [.36, 1.0], [.5, .88], [.56, .62], [.52, .34], [.4, .12], [.24, .01], [.1, .02], [0, .08]];
export function apples(c) {
  const skin = matte(c.body, { rough: .3, wet: .6, bump: .2 }), leafM = matte(c.trim, { rough: .6, double: true, sheen: .3 }), stemM = matte(c.detail, { rough: .9 }), flesh = matte('#f2ead0', { rough: .7, double: true }), S = .62, P = smooth(appleP, 48);
  const A = turned(P, skin, { sm: 0, s: S, p: [-.4, 0, -.1] }), B = turned(P, skin, { sm: 0, s: S * .92, r: [0, 0, -.28], p: [.42, 0, .02] });
  const stems = [rod([[0, .5, 0], [.02, .68, 0], [.08, .8, 0]], .018, stemM, { n: 10, sides: 6, p: [-.4, 0, -.1] }), rod([[0, .46, 0], [.03, .62, 0], [.0, .74, .04]], .016, stemM, { n: 10, sides: 6, r: [0, 0, -.28], p: [.42, 0, .02] })];
  const leafO = [[0, 0], [.1, .08], [.24, .09], [.38, 0], [.24, -.09], [.1, -.07]], leafZ = (x, y) => .05 * Math.sin(x * 7) - 2.4 * y * y;
  const leaves = [fan(leafO, leafZ, leafM, { nu: 40, nv: 6, cx: .18, r: [.3, 0, .5], p: [-.38, .66, -.1] }), rod([[0, 0, 0], [.36, 0, .05]], .006, leafM, { sides: 5, r: [.3, 0, .5], p: [-.38, .665, -.1] })];
  // A half lies cut side up in front, tipped toward the eye.
  const put = m => { m.geometry.rotateZ(-H2); m.geometry.rotateX(.62); m.geometry.rotateY(-.5); m.geometry.translate(-.3, .347, .62); return m; };
  const half = put(M(new THREE.LatheGeometry(outward(P).map(p => new THREE.Vector2(p[0], p[1])), 36, 0, Math.PI), skin, { s: S }));
  const cut = put(slab(P.map(p => [p[0] * S, p[1] * S]).concat(P.slice().reverse().map(p => [-p[0] * S, p[1] * S])), .008, 0, flesh, { r: [0, H2, 0] }));
  const seeds = [-1, 1].map(k => put(ball(.03, stemM, { s: [.4, 1.7, 1], p: [-.008, .3, k * .06] })));
  const core = put(M(new THREE.TorusGeometry(.1, .006, 6, 24), matte('#d8cba0', { rough: .8 }), { s: [1, 1.5, 1], r: [0, H2, 0], p: [-.006, .31, 0] }));
  return item('Apples', [part('apples', 'body', [0, 0, 0], [A, B, half]), part('leaf', 'trim', [0, .6, 0], leaves), part('stems and seeds', 'detail', [0, .4, .4], stems.concat(seeds)), part('flesh', '', [0, .2, .8], [cut, core])], { stand: true, yaw: .15, el: 22 });
}

export function grapes(c) {
  const skin = matte(c.body, { rough: .28, wet: .7, sheen: .7 }), leafM = matte(c.trim, { rough: .6, double: true, sheen: .3 }), stemM = matte(c.detail, { rough: .9, bump: .8 }), r = rng(11), at = [];
  // The bunch narrows to a point; grapes are packed in until no more fit.
  for (let n = 0; n < 6000 && at.length < 64; n++) {
    const s = r(), y = .62 - s * 1.75, R = .62 * Math.pow(1 - s, .6) * Math.min(1, .35 + s * 4), a = r() * TAU, d = R * (.45 + .55 * r()), p = V(Math.cos(a) * d, y, Math.sin(a) * d * .8);
    if (at.every(q => q.p.distanceTo(p) > .2)) { const k = .9 + r() * .22; at.push({ p: p, s: [k, k * 1.14, k] }); }
  }
  const bunch = many(new THREE.SphereGeometry(.125, 22, 16), skin, at);
  const stem = rod([[0, -.3, 0], [0, .5, 0], [.02, .84, 0], [-.04, 1.02, 0]], s => .025 + .025 * s, stemM, { n: 20, sides: 8 }), vine = rod([[-.36, .98, 0], [0, 1.03, 0], [.36, .96, 0]], .045, stemM, { n: 12, sides: 8 });
  const coil = []; for (let j = 0; j <= 48; j++) { const a = j / 48 * TAU * 2.5; coil.push(V(.3 + j * .007 + .07 * Math.cos(a), .96 + .07 * Math.sin(a), .1 * j / 48)); }
  const tendril = rod(coil, s => .014 - .008 * s, stemM, { sides: 6 });
  // One leaf of five lobes, still on the vine.
  const lo = []; for (let i = 0; i < 40; i++) { const a = i / 40 * TAU, rr = .5 * (.62 + .38 * Math.pow(Math.abs(Math.cos(a * 2.5 + H2 * 2.5)), .7)) * (1 - .5 * Math.exp(-Math.pow((a - H2) / .25, 2))); lo.push([Math.cos(a) * rr, Math.sin(a) * rr]); }
  const lr = [.25, .3, -.5], lp = [-.5, .56, -.02], leaf = fan(lo, (x, y) => -.35 * (x * x + y * y) + .025 * Math.sin(x * 16) * Math.sin(y * 13), leafM, { nu: 80, nv: 10, tension: .3, r: lr, p: lp });
  const lz = (x, y) => -.35 * (x * x + y * y) + .006, veins = [-90, -18, 54, 126, 198].map(d => { const a = d / 180 * Math.PI, x = Math.cos(a), y = Math.sin(a); return rod([[0, .15, lz(0, .15)], [x * .2, .08 + y * .2, lz(x * .2, .08 + y * .2)], [x * .42, y * .42, lz(x * .42, y * .42)]], .007, leafM, { n: 8, sides: 5, r: lr, p: lp }); });
  const petiole = rod([[-.34, .98, 0], [-.4, .84, 0], [-.43, .7, -.02]], .012, stemM, { n: 8, sides: 6 });
  return item('Grapes', [part('grapes', 'body', [0, -.3, 0], [bunch]), part('leaf', 'trim', [-.5, .3, 0], [leaf].concat(veins)), part('stem', 'detail', [0, .6, 0], [stem, vine, tendril, petiole])], { yaw: .2, fill: .95 });
}

export function mushrooms(c) {
  const capM = matte(c.body, { rough: .34, wet: .5 }), stalk = matte(c.trim, { rough: .8, bump: .5 }), spot = matte(c.detail, { rough: .7 }), moss = matte('#4a5a2c', { rough: 1, grain: 'rock', bump: 1.4 }), gill = matte('#d8cdb4', { rough: .9, double: true }), r = rng(4);
  const caps = [], stalks = [], unders = [], spots = [];
  // Each grows from the same clump and leans its own way; the spots follow the cap.
  for (const [x, z, h, R, lean] of [[0, 0, 1.1, .62, -.08], [-.66, .2, .72, .4, .32], [.6, .28, .46, .27, -.36], [.16, .6, .3, .18, .12]]) {
    const o = { r: [0, 0, lean], p: [x, 0, z] }, y0 = h * .92, ry = h * .08 + R * .62, dome = [];
    for (let k = 0; k <= 14; k++) { const a = k / 14 * H2; dome.push([Math.max(1e-4, R * Math.cos(a)), y0 + ry * Math.sin(a)]); }
    stalks.push(turned([[0, -.1], [R * .34, -.1], [R * .26, h * .2], [R * .2, h * .6], [R * .22, h * .96]], stalk, Object.assign({ sm: 30 }, o)), turned([[R * .2, h * .74], [R * .34, h * .64], [R * .37, h * .58]], stalk, Object.assign({ sm: 0, seg: 28 }, o)));
    caps.push(turned(dome, capM, Object.assign({ sm: 0 }, o)), turned([[R, y0], [R * 1.02, y0 - .02 * R], [R * .96, y0 - .05 * R]], capM, Object.assign({ sm: 0 }, o)));
    unders.push(turned([[R * .2, h * .97], [R * .6, y0 - .12 * R], [R * .97, y0 - .05 * R]], gill, Object.assign({ sm: 0, seg: 40 }, o)));
    const cs = Math.cos(lean), sn = Math.sin(lean), n = Math.round(5 + R * 12);
    for (let i = 0; i < n; i++) { const th = .15 + r() * 1.05, ph = r() * TAU, lx = R * Math.sin(th) * Math.cos(ph), ly = y0 + ry * Math.cos(th), lz = R * Math.sin(th) * Math.sin(ph), nx = Math.sin(th) * Math.cos(ph) / R, ny = Math.cos(th) / ry, nz = Math.sin(th) * Math.sin(ph) / R, sz = R * (.07 + r() * .08);
      spots.push({ p: V(x + lx * cs - ly * sn, lx * sn + ly * cs, z + lz), q: aim(V(nx * cs - ny * sn, nx * sn + ny * cs, nz)), s: [sz, sz * .28, sz] }); }
  }
  const spotsM = many(new THREE.SphereGeometry(1, 14, 10), spot, spots);
  const mound = blob([1.15, .14, .85], v => 1 + .12 * Math.sin(v.x * 9) * Math.sin(v.z * 7), moss, { p: [0, .0, .2] });
  return item('Mushrooms', [part('caps', 'body', [0, .5, 0], caps), part('stalks', 'trim', [0, -.2, .3], stalks), part('spots', 'detail', [0, .7, .5], [spotsM]), part('moss', '', [0, -.4, 0], [mound].concat(unders))], { stand: true, yaw: .2, el: 14 });
}

export function teapot(c, t) {
  const ph = (t || 0) * TAU, glaze = matte(c.body, { rough: .22, wet: .9, bump: .1 }), cane = wood(c.trim, { polish: .5 }), paint = matte(c.detail, { rough: .3, wet: .8 });
  const P = smooth([[0, .04], [.36, .04], [.4, 0], [.44, .04], [.62, .3], [.68, .56], [.6, .84], [.42, 1.0], [.3, 1.04], [.3, 1.08]], 64), R = radiusAt(P.slice(8));
  const pot = turned(P, glaze, { sm: 0 }), lid = turned([[.31, 1.07], [.3, 1.1], [.2, 1.18], [.08, 1.22], [0, 1.23]], glaze, { sm: 30 }), neck = cyl(.03, .025, .06, paint, { p: [0, 1.25, 0] }), knob = ball(.065, paint, { p: [0, 1.31, 0] });
  const spout = rod([[.5, .46, 0], [.82, .54, 0], [1.0, .8, 0], [1.12, 1.04, 0]], s => .14 - .08 * s, glaze, { n: 30, sides: 16 }), mouth = ball(.05, dark(), { p: [1.125, 1.05, 0], s: [1, .5, 1] });
  const lugs = [-1, 1].map(k => torus(.055, .022, glaze, { p: [k * .43, 1.02, 0] }));
  // A cane handle over the top, as on a pot meant to hang over a fire.
  const handle = rod([[-.43, 1.02, 0], [-.52, 1.32, 0], [-.32, 1.68, 0], [0, 1.78, 0], [.32, 1.68, 0], [.52, 1.32, 0], [.43, 1.02, 0]], .036, cane, { n: 48 });
  const binds = [-.3, -.15, 0, .15, .3].map(x => torus(.04, .012, cane, { r: [0, H2, Math.atan(-2.2 * x)], p: [x, 1.78 - x * x * 1.1, 0] }));
  const dots = many(new THREE.SphereGeometry(1, 14, 10), paint, Array.from({ length: 16 }, (_, i) => { const a = i / 16 * TAU, rr = R(.56) + .002; return { p: V(Math.cos(a) * rr, .56, Math.sin(a) * rr), q: aim(V(Math.cos(a), 0, Math.sin(a))), s: [.05, .012, .05] }; }));
  const lines = [.42, .7].map(y => band(R(y) - .006, y, .026, .01, paint));
  const steam = [0, 1].map(i => { const pts = []; for (let j = 0; j <= 20; j++) { const s = j / 20; pts.push(V(1.14 + .1 * s + Math.sin(s * 5 + i * 2 - ph) * .09 * s, 1.07 + s * .8, Math.cos(s * 4 + i - ph) * .08 * s)); } return rod(pts, s => .012 + .07 * s, ghost('#ffffff', .05), { sides: 8, nofit: true }); });
  return item('Teapot', [part('pot', 'body', [0, 0, 0], [pot, lid, spout, mouth].concat(lugs)), part('handle', 'trim', [0, .6, 0], [handle].concat(binds)), part('painting', 'detail', [0, 0, .8], [dots, neck, knob].concat(lines)), part('steam', '', [.5, .5, 0], steam)], { stand: true, yaw: .3, el: 14 });
}

export function keg(c) {
  const oak = wood(c.body, { bump: 1 }), iron = metal(c.trim, { kind: 'forged', rough: .5 }), br = metal(c.detail, { kind: 'cast', rough: .26 }), ale = liquid('#b8741a', { lit: .3 }), L = .82, o = { r: [H2, 0, 0], p: [0, L, 0] };
  // The barrel lies on a cradle, head toward the eye; eighteen staves show as flats.
  const P = smooth([[.5, -.8], [.6, -.5], [.66, 0], [.6, .5], [.5, .8]], 40), R = radiusAt(P), g = lathe(P, 18).toNonIndexed(); g.computeVertexNormals();
  const oakIn = wood(c.body, { bump: 1 }); oakIn.side = THREE.BackSide;
  const barrel = M(g, oak, o), inner = M(lathe(P.map(p => [p[0] - .03, p[1]]), 18), oakIn, o);
  const heads = [-1, 1].map(k => cyl(.5, .5, .03, oak, { r: [H2, 0, 0], p: [0, L, k * .74] })), seams = [-.2, .16].map(x => rbox(.008, 2 * Math.sqrt(.25 - x * x) - .02, .006, .002, dark(), { p: [x, L, .757] }));
  const hoops = [[-.76, .028], [-.42, .03], [.42, .03], [.76, .028]].map(([y, w]) => band(R(y) + .004, y, .09, w * .6, iron, o));
  const tap = [turned([[.05, 0], [.05, .2], [.04, .22], [.04, .36], [.06, .38]], br, { sm: 0, seg: 20, r: [H2, 0, 0], p: [0, L - .28, .75] }), cyl(.03, .035, .12, br, { p: [0, L - .35, 1.08] }), rod([[0, L - .26, .98], [0, L - .12, .98]], .016, br, { sides: 8 }), rbox(.16, .03, .03, .01, br, { p: [0, L - .11, .98] })];
  const bung = cyl(.07, .06, .08, br, { p: [0, L + .68, 0] });
  const drip = [blob([.022, .034, .022], v => 1 + .3 * Math.max(0, v.y), ale, { p: [0, L - .45, 1.08] }), blob([.1, .012, .08], null, ale, { p: [.02, .008, 1.08] })];
  // The cradle: two boards cut to the barrel's curve, pegged to two rails.
  const arc = []; for (let i = 0; i <= 12; i++) { const a = (205 + i / 12 * 130) / 180 * Math.PI; arc.push([Math.cos(a) * .63, L + Math.sin(a) * .63]); }
  const board = [[-.76, 0], [-.76, .46], [-.62, .56]].concat(arc, [[.62, .56], [.76, .46], [.76, 0]]);
  const cradle = [-1, 1].map(k => slab(board, .1, .015, oak, { p: [0, 0, k * .44] })).concat([-1, 1].map(k => rbox(.1, .08, 1.1, .015, oak, { p: [k * .6, .05, 0] })));
  return item('Keg', [part('barrel and cradle', 'body', [0, 0, 0], [barrel, inner].concat(heads, seams, cradle)), part('hoops', 'trim', [0, .5, 0], hoops), part('tap and bung', 'detail', [0, 0, .8], tap.concat([bung])), part('ale', '', [0, -.3, .8], drip)], { stand: true, yaw: .6, el: 14 });
}
