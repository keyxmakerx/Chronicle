// A vial of poison, a treasure map, a cauldron, coral, amber and a sabre.
import { THREE, V, M, X, Y, Z, part, item, turned, rod, ball, torus, cyl, rbox, slab, blob, sheet, many, aim, blade, band, wrap, drawn, strands, loft, ring, tube, path, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, cloth, paper, matte, glass, liquid, gem, shell, rock, glow, ghost, dark } from '../mats.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
const facet = m => { m.flatShading = true; for (const l of m.userData.layers || []) l.mat.flatShading = true; return m; };

export function poison(c, t) {
  const tt = t || 0, ph = tt * TAU, gl = facet(glass(c.body, { tint: .4 })), venom = liquid(c.detail, { lit: .5 + .12 * Math.sin(ph), inner: 1.6 }), pewter = metal(c.trim, { kind: 'cast', rough: .4 }), cordM = matte('#3a2a20', { rough: .9, bump: .8 }), r = rng(31);
  // Six flat sides, so it can be told from a healing draught by touch in the dark.
  const body = turned([[0, .04], [.34, .02], [.4, .06], [.44, .2], [.4, .62], [.24, .86], [.13, .96], [.115, 1.2], [.15, 1.23], [.15, 1.27], [.1, 1.28]], gl, { sm: 0, seg: 6 });
  const fill = turned([[0, .08], [.33, .07], [.39, .2], [.37, .52], [0, .52]], venom, { sm: 0, seg: 6 });
  body.geometry.rotateY(Math.PI / 6); fill.geometry.rotateY(Math.PI / 6);
  const bubbles = Array.from({ length: 9 }, () => { const a = r() * TAU, d = r() * .26, h = (r() + tt) % 1; return ball(.012 + r() * .014, glow(c.detail, 2.2 * Math.min(1, h * 6) * (1 - h * .5), { add: true }), { p: [Math.cos(a) * d, .1 + h * .4, Math.sin(a) * d], seg: 8, seg2: 6 }); });
  // A pewter collar with three claws down the shoulders holds a stopper cast as a skull.
  const collar = [band(.125, 1.0, .08, .014, pewter), band(.16, 1.25, .05, .01, pewter)], claws = [];
  for (let i = 0; i < 3; i++) { const a = i * TAU / 3 + .52, x = Math.cos(a), z = Math.sin(a); claws.push(rod([[x * .135, 1.0, z * .135], [x * .22, .9, z * .22], [x * .36, .7, z * .36], [x * .39, .56, z * .39]], s => .022 - .012 * s, pewter, { n: 14, sides: 6 })); }
  const skull = [blob([.13, .14, .15], v => 1 + (v.y < -.2 ? -.25 * Math.min(1, (-.2 - v.y) * 2) : 0), pewter, { p: [0, 1.46, 0] }), cyl(.09, .1, .1, pewter, { p: [0, 1.31, .02], seg: 16 })];
  const sockets = [-1, 1].map(k => ball(.036, dark(), { p: [k * .05, 1.46, .125], s: [1, 1.1, .5] })).concat([ball(.016, dark(), { p: [0, 1.4, .135], s: [1, 1.4, .5] })]);
  const teeth = many(new THREE.BoxGeometry(.006, .06, .01), dark(), [-.045, -.015, .015, .045].map(x => ({ p: [x, 1.31, .115] })));
  // The warning is tied to its neck, and a spilled drop is eating into the table.
  const label = drawn(128, 160, (x, w, h) => { x.strokeStyle = x.fillStyle = '#2a1a14'; x.lineWidth = 5; x.lineCap = 'round'; x.beginPath(); x.moveTo(30, 96); x.lineTo(98, 142); x.moveTo(98, 96); x.lineTo(30, 142); x.stroke(); x.beginPath(); x.arc(64, 62, 30, 0, TAU); x.fill(); x.fillRect(48, 82, 32, 20); x.fillStyle = '#e4d8b8'; x.beginPath(); x.arc(52, 62, 8, 0, TAU); x.arc(76, 62, 8, 0, TAU); x.fill(); x.fillRect(57, 90, 3, 12); x.fillRect(63, 90, 3, 12); x.fillRect(69, 90, 3, 12); }, [.88, .82, .68]);
  const tag = sheet((u, v) => V(.5 + u * .34, .012 + .03 * Math.sin(u * 3) * (1 - v), .42 + v * .42 - u * .1), 8, 8, paper('#ffffff', { map: label }));
  const tie = [torus(.13, .011, cordM, { r: [H2, 0, 0], p: [0, 1.1, 0] }), rod([[.12, 1.1, .04], [.3, 1.02, .16], [.48, .5, .34], [.56, .06, .44], [.6, .016, .5]], .01, cordM, { n: 30, sides: 6 })];
  const drop = [blob([.11, .014, .08], null, venom, { p: [-.62, .012, .5] }), blob([.05, .012, .04], null, venom, { p: [-.46, .01, .62] })];
  const fume = [0, 1].map(i => { const pts = []; for (let j = 0; j <= 20; j++) { const s = j / 20; pts.push(V(-.62 + Math.sin(s * 5 + i * 2 - ph) * .06 * s + i * .04, .03 + s * .5, .5 + Math.cos(s * 4 + i - ph) * .05 * s)); } const m = rod(pts, s => .01 + .045 * s, ghost(c.detail, .06), { sides: 8 }); m.userData.nofit = true; return m; });
  return item('Vial of poison', [part('vial', 'body', [0, 0, 0], [body]), part('venom', 'detail', [0, 0, .9], [fill].concat(bubbles, drop, fume)), part('collar and stopper', 'trim', [0, .5, 0], collar.concat(claws, skull, sockets, [teeth])), part('warning', '', [.5, 0, .3], tie.concat([tag]))], { stand: true, yaw: .25, el: 14, lights: [{ p: [0, .4, 0], color: c.detail, power: .9 }], halo: .7 });
}

export function map(c) {
  const n = makeNoise(41, 16), brass = metal(c.trim, { kind: 'cast', rough: .3 }), silk = cloth(c.detail, { sheen: 1, bump: .3 }), sealM = matte(c.detail, { rough: .4, wet: .4 }), W = 2.1, Hh = 1.45;
  // The chart itself: a coast, hills, a road of dashes and the place to dig.
  const chart = drawn(768, 512, (x, w, h) => {
    const r = rng(77), ink = '#3a2618';
    x.strokeStyle = ink; x.lineWidth = 3; x.strokeRect(22, 22, w - 44, h - 44); x.lineWidth = 1; x.strokeRect(30, 30, w - 60, h - 60);
    x.lineWidth = 2.5; x.lineJoin = 'round'; x.beginPath(); for (let i = 0; i <= 80; i++) { const a = i / 80 * TAU, rr = 150 + 46 * Math.sin(a * 3 + 1) + 30 * Math.sin(a * 7) + (n(i * .4, 2) - .5) * 40, px = 330 + Math.cos(a) * rr * 1.5, py = 260 + Math.sin(a) * rr * .95; i ? x.lineTo(px, py) : x.moveTo(px, py); } x.closePath(); x.fillStyle = 'rgba(150,110,60,.2)'; x.fill(); x.stroke();
    x.lineWidth = 1; for (let k = 1; k < 4; k++) { x.globalAlpha = .35 - k * .08; x.beginPath(); for (let i = 0; i <= 80; i++) { const a = i / 80 * TAU, rr = 150 + 46 * Math.sin(a * 3 + 1) + 30 * Math.sin(a * 7) + k * 12, px = 330 + Math.cos(a) * rr * 1.5, py = 260 + Math.sin(a) * rr * .95; i ? x.lineTo(px, py) : x.moveTo(px, py); } x.closePath(); x.stroke(); } x.globalAlpha = 1;
    x.lineWidth = 2; for (const [mx, my] of [[250, 200], [290, 180], [330, 205], [215, 235], [300, 240]]) { x.beginPath(); x.moveTo(mx - 22, my + 18); x.lineTo(mx, my - 18); x.lineTo(mx + 22, my + 18); x.stroke(); x.beginPath(); x.moveTo(mx, my - 18); x.lineTo(mx + 6, my + 4); x.stroke(); }
    for (let i = 0; i < 12; i++) { const tx = 400 + r() * 110, ty = 280 + r() * 90; x.beginPath(); x.arc(tx, ty, 7, 0, TAU); x.stroke(); x.beginPath(); x.moveTo(tx, ty + 7); x.lineTo(tx, ty + 15); x.stroke(); }
    x.setLineDash([12, 9]); x.lineWidth = 3; x.beginPath(); x.moveTo(130, 330); x.bezierCurveTo(220, 380, 300, 300, 380, 330); x.bezierCurveTo(450, 356, 470, 240, 540, 222); x.stroke(); x.setLineDash([]);
    x.strokeStyle = '#a3262e'; x.lineWidth = 7; x.lineCap = 'round'; x.beginPath(); x.moveTo(528, 200); x.lineTo(566, 238); x.moveTo(566, 200); x.lineTo(528, 238); x.stroke();
    x.strokeStyle = ink; x.lineWidth = 2; x.beginPath(); x.arc(655, 400, 44, 0, TAU); x.stroke(); x.fillStyle = ink; for (let i = 0; i < 4; i++) { const a = i * H2; x.beginPath(); x.moveTo(655 + Math.cos(a) * 60, 400 + Math.sin(a) * 60); x.lineTo(655 + Math.cos(a + 1.4) * 10, 400 + Math.sin(a + 1.4) * 10); x.lineTo(655 + Math.cos(a - 1.4) * 10, 400 + Math.sin(a - 1.4) * 10); x.fill(); }
    x.lineWidth = 1.5; for (let i = 0; i < 7; i++) { const wx = 560 + r() * 150, wy = 70 + r() * 240; x.beginPath(); x.moveTo(wx, wy); x.quadraticCurveTo(wx + 9, wy - 8, wx + 18, wy); x.quadraticCurveTo(wx + 27, wy + 8, wx + 36, wy); x.stroke(); }
    for (let i = 0; i < 40; i++) { x.fillStyle = 'rgba(110,70,30,' + (.03 + r() * .06) + ')'; x.beginPath(); x.arc(r() * w, r() * h, 10 + r() * 50, 0, TAU); x.fill(); }
  }, [.9, .82, .62]);
  const sheetM = paper(c.body, { map: chart });
  // It has been rolled for years: both ends still curl up, and it never lies quite flat.
  const curl = (d, k) => { const a = d * k; return [Math.sin(a) / k, (1 - Math.cos(a)) / k]; };
  const chartS = sheet((u, v) => { const x = (u - .5) * W, y = (v - .5) * Hh, e = .78, s = Math.sign(x), ax = Math.abs(x), over = Math.max(0, ax - e), q = curl(over, 5 + (s > 0 ? 3 : 0)), lift = .05 * Math.pow(Math.abs(u - .5) * 2, 3) * Math.pow(Math.abs(v - .5) * 2, 2); return V(s * (Math.min(ax, e) + q[0]), y, .01 + q[1] + lift + (fbm(n, u * 3, v * 3, 2) - .5) * .03); }, 90, 30, sheetM);
  // Dividers left lying on it, open to a day's march.
  const leg = (a, len) => { const m = turned([[.03, 0], [.034, .1], [.022, .16], [.016, len * .8], [.002, len]], brass, { sm: 30, seg: 14 }); m.geometry.rotateZ(a); return m; }, dv = [leg(Math.PI + .28, .7), leg(Math.PI - .28, .7), cyl(.05, .05, .04, brass, { r: [H2, 0, 0] }), turned([[.02, 0], [.02, .12], [.04, .15], [.04, .2], [0, .22]], brass, { sm: 0, seg: 14 })];
  for (const m of dv) { m.geometry.rotateZ(-.9); m.geometry.translate(-.32, .2, .075); }
  const seal = [turned([[0, .034], [.07, .034], [.1, .026], [.125, .012], [.13, 0]], sealM, { sm: 20, r: [H2, 0, 0], p: [.56, -.56, .012] }), torus(.05, .008, sealM, { p: [.56, -.56, .05] })];
  const ribbon = sheet((u, v) => { const s = u * 1.5, x = .56 - s * .5 + .08 * Math.sin(s * 4), y = -.56 - s * .22 - .05 * Math.sin(s * 3); return V(x + (v - .5) * .02, y + (v - .5) * .11, .014 + .03 * Math.abs(Math.sin(s * 5)) * Math.min(1, s * 3)); }, 40, 3, silk);
  return item('Treasure map', [part('chart', 'body', [0, 0, -.3], [chartS]), part('dividers', 'trim', [-.3, .4, .5], dv), part('seal and ribbon', 'detail', [.5, -.5, .4], seal.concat([ribbon]))], { yaw: .25, fill: .94 });
}

export function cauldron(c, t) {
  const tt = t || 0, ph = tt * TAU, iron = metal(c.body, { kind: 'forged', rough: .75, bump: 1.4 }), wd = wood(c.trim, { bump: .9 }), brew = liquid(c.detail, { lit: .75 + .1 * Math.sin(ph), inner: .6 }), r = rng(17), n = makeNoise(29, 16);
  const pot = turned([[0, .17], [.36, .19], [.66, .36], [.8, .62], [.76, .88], [.63, 1.02], [.62, 1.06], [.7, 1.1], [.71, 1.14], [.6, 1.14], [.55, 1.06], [.56, 1.0], [.68, .86], [.71, .62], [.58, .4], [0, .3]], iron, { sm: 120, seg: 72 });
  const legs = [], lugs = [];
  for (let i = 0; i < 3; i++) { const a = i * TAU / 3 + .4, x = Math.cos(a), z = Math.sin(a); legs.push(rod([[x * .46, .34, z * .46], [x * .56, .16, z * .56], [x * .6, 0, z * .6]], s => .07 - .02 * s, iron, { n: 10, sides: 10 })); }
  for (const k of [-1, 1]) lugs.push(torus(.08, .024, iron, { r: [0, 0, H2], p: [k * .74, 1.0, 0] }), torus(.11, .02, iron, { r: [.5 * k, 0, 0], p: [k * .82, .9, 0] }));
  // The brew rolls: bubbles swell where it is hottest and break, and what comes off it rises.
  const top = sheet((u, v) => { const a = u * TAU, d = v * .6; return V(Math.cos(a) * d, .96 + .012 * Math.sin(d * 14 - ph) + (n(Math.cos(a) * d * 6 + 4, Math.sin(a) * d * 6) - .5) * .02, Math.sin(a) * d); }, 60, 16, brew);
  const bub = Array.from({ length: 14 }, () => { const a = r() * TAU, d = Math.sqrt(r()) * .5, u = (tt * (1 + Math.floor(r() * 2)) + r()) % 1, s = (.035 + r() * .07) * Math.sin(Math.min(1, u * 1.15) * Math.PI * .5) * (u > .87 ? 0 : 1); return ball(Math.max(.001, s), brew, { p: [Math.cos(a) * d, .955, Math.sin(a) * d], seg: 16, seg2: 12 }); });
  const steam = [0, 1, 2, 3, 4].map(i => { const pts = []; for (let j = 0; j <= 24; j++) { const s = j / 24; pts.push(V((i - 2) * .13 + Math.sin(s * 5 + i * 2 - ph) * .12 * s, .98 + s * (.7 + i % 2 * .25), Math.cos(s * 4 + i * 1.7 - ph) * .1 * s + (i % 2 - .5) * .2)); } const m = rod(pts, s => .012 + .075 * s * (1.2 - s), ghost(c.detail, .028), { sides: 8 }); m.userData.nofit = true; return m; });
  const motes = Array.from({ length: 10 }, () => { const a = r() * TAU, d = r() * .4, h = (r() + tt) % 1, m = ball(.009 + r() * .008, glow(c.detail, 2.4 * Math.min(1, h * 6) * (1 - h), { add: true }), { p: [Math.cos(a) * d, 1.0 + h * .7, Math.sin(a) * d], seg: 8, seg2: 6 }); m.userData.nofit = true; return m; });
  // A ladle stands in it, leaning on the rim.
  const ladle = [turned([[.03, 0], [.026, .9], [.036, .96], [0, 1.0]], wd, { sm: 20, seg: 14 }), ball(.11, wd, { p: [0, -.04, 0], s: [1, .7, 1] })];
  for (const m of ladle) { m.geometry.rotateZ(-.5); m.geometry.translate(.3, .98, .12); }
  return item('Cauldron', [part('pot', 'body', [0, 0, 0], [pot].concat(legs, lugs)), part('ladle', 'trim', [.5, .5, 0], ladle), part('brew', 'detail', [0, .7, 0], [top].concat(bub, steam, motes))], { stand: true, yaw: .3, el: 24, lights: [{ p: [0, 1.25, 0], color: c.detail, power: 2.2, flick: .12 }], halo: .8 });
}

export function coral(c) {
  const r = rng(52), n = makeNoise(13, 16), flesh = matte(c.body, { rough: .55, bump: 1.4, wet: .2, grain: 'rock' }), rk = rock(c.trim, { bump: 2.2 }), tipM = matte(c.detail, { rough: .5, wet: .3 });
  const base = blob([.6, .2, .46], v => 1 + (fbm(n, v.x * 2.4 + 3, v.z * 2.4 + v.y * 2, 4) - .5) * .8, rk, { p: [0, .1, 0], seg: 90, seg2: 50 });
  // It grows the way a tree does, but every fork is two fingers reaching for the light.
  const limbs = [], tips = [];
  const grow = (p, d, len, rad, depth) => {
    const mid = p.clone().addScaledVector(d, len * .5).add(V((r() - .5) * len * .2, 0, (r() - .5) * len * .2)), end = p.clone().addScaledVector(d, len);
    limbs.push(rod([p, mid, end], s => rad * (1 - s * .28), flesh, { n: 8, sides: 9 }));
    if (depth === 0 || rad < .022) { tips.push({ p: end, s: rad * 1.25 }); return; }
    const k = depth > 3 || r() > .35 ? 2 : 3;
    for (let i = 0; i < k; i++) { const a = r() * TAU, sp = .38 + r() * .34, nd = d.clone().add(V(Math.cos(a) * sp, .12, Math.sin(a) * sp * .7)).normalize(); grow(end, nd, len * (.66 + r() * .2), rad * .72, depth - 1); }
  };
  for (const [x, z, lean] of [[-.16, 0, -.25], [.2, .06, .3], [.0, -.12, 0]]) grow(V(x, .2, z), V(lean, 1, (r() - .5) * .2).normalize(), .4, .085, 5);
  const polyps = many(new THREE.SphereGeometry(1, 10, 8), tipM, tips.map(q => ({ p: q.p, s: q.s })));
  const shellM = shell('#d8c8b0', { irid: .5 }), barn = [[.42, .2, .26], [-.44, .17, .2], [.3, .12, -.3]].map(([x, y, z]) => turned([[.07, 0], [.05, .05], [.024, .07], [.02, .05], [0, .05]], shellM, { sm: 0, seg: 10, p: [x, y, z] }));
  return item('Branch of coral', [part('rock', 'trim', [0, -.4, 0], [base].concat(barn)), part('coral', 'body', [0, .2, 0], limbs), part('tips', 'detail', [0, .6, .4], [polyps])], { stand: true, yaw: .3, el: 16 });
}

export function amber(c) {
  const n = makeNoise(61, 16), resin = gem(c.body, { round: true, body: .42, pale: .35, lit: .32, inner: 1.5, edges: 0, frost: .06, fire: 1.6, mirror: .12 }), bark = wood(c.trim, { bump: 2.4 }), wing = shell(c.detail, { irid: 1, metal: .6 }), r = rng(5);
  const lump = (size, p, seed) => blob(size, v => 1 + (fbm(n, v.x * 1.4 + seed, v.y * 1.4 + v.z * 1.2, 3) - .5) * .5, resin, { p: p, seg: 80, seg2: 56 });
  const big = lump([.62, .44, .5], [0, .5, 0], 3), small = [lump([.2, .15, .17], [.86, .15, .34], 9), lump([.13, .1, .12], [-.8, .1, .5], 14)];
  // What was walking on the tree the day the resin ran: a beetle, whole, with its legs still spread.
  const bug = [ball(.1, wing, { p: [0, .52, .1], s: [.8, .55, 1.2] }), ball(.05, dark(), { p: [0, .52, .24], s: [1, .8, 1] }), rod([[0, .56, 0], [0, .575, .1], [0, .56, .2]], .006, dark(), { sides: 4 })];
  for (let i = 0; i < 6; i++) { const k = i % 2 ? 1 : -1, z = .02 + Math.floor(i / 2) * .08; bug.push(rod([[k * .06, .5, z], [k * .15, .54, z + (Math.floor(i / 2) - 1) * .05], [k * .2, .45, z + (Math.floor(i / 2) - 1) * .09]], .007, dark(), { n: 8, sides: 4 })); }
  for (const k of [-1, 1]) bug.push(rod([[k * .02, .53, .28], [k * .06, .57, .36], [k * .12, .56, .4]], .004, dark(), { n: 6, sides: 4 }));
  for (const m of bug) { m.geometry.rotateY(.7); m.geometry.rotateZ(.2); m.geometry.translate(.02, 0, .08); }
  // It lies on the piece of bark it bled from.
  const slabB = sheet((u, v) => { const x = (u - .5) * 2.3, z = (v - .5) * 1.5, e = Math.max(Math.abs(u - .5), Math.abs(v - .5)) * 2; return V(x + .08 * Math.sin(v * 9), .03 + .1 * Math.pow(Math.abs(v - .5) * 2, 2) + (fbm(n, u * 5, v * 3, 3) - .5) * .09, z + .1 * Math.sin(u * 7) * (v > .5 ? 1 : -1)); }, 60, 30, bark, { uvx: 2, uvy: 1 });
  slabB.material.side = THREE.DoubleSide;
  return item('Amber', [part('bark', 'trim', [0, -.4, 0], [slabB]), part('amber', 'body', [0, .2, 0], [big].concat(small)), part('what is caught in it', 'detail', [0, .2, .9], bug)], { stand: true, yaw: .3, el: 26 });
}

export function sabre(c) {
  const steel = metal(c.body, { kind: 'steel', rough: .26 }), brass = metal(c.trim, { kind: 'cast', rough: .3 }), hide = leather(c.detail, { bump: 1.1 }), silk = cloth(c.detail, { sheen: 1, bump: .3 }), L = 1.75, r = rng(4);
  // One long curve: the blade widens toward the point and is clipped there, so the weight sits behind the cut.
  const bend = y => y > 0 ? .34 * Math.pow(y / L, 2.1) * L * .55 : 0;
  const g = blade(L, s => (.062 + .03 * Math.min(1, Math.max(0, (s - .45) / .4))) * (s > .9 ? Math.pow(1 - (s - .9) / .1, .6) : 1), s => .013 * (1 - s * .55), { fuller: s => s < .72 ? .016 : 0 });
  { const p = g.attributes.position; for (let i = 0; i < p.count; i++) { const y = p.getY(i), x = p.getX(i); p.setX(i, x * (x > 0 ? .55 : 1.3) + bend(y)); } g.computeVertexNormals(); }
  const bladeM = M(g, steel);
  const guard = [rbox(.3, .05, .07, .02, brass, { p: [0, -.02, 0] }), ball(.04, brass, { p: [.17, -.02, 0], s: [1, 1.2, 1] })];
  // The knuckle bow sweeps from the guard round the hand to the pommel.
  const bow = rod([[-.15, -.02, 0], [-.24, -.06, 0], [-.27, -.22, 0], [-.2, -.42, 0], [-.07, -.5, 0]], s => .02 + .012 * Math.sin(s * Math.PI), brass, { n: 30, sides: 10, s: [1, 1, 1.5] });
  const grip = wrap(-.46, -.05, y => .04 + .008 * Math.sin((y + .46) / .41 * Math.PI), hide, { turns: 16, cord: .009 });
  const pommel = turned([[.04, -.46], [.056, -.5], [.06, -.55], [.04, -.6], [0, -.61]], brass, { sm: 30, seg: 24 });
  const loop = rod([[0, -.6, 0], [.1, -.7, .01], [.2, -.66, .02], [.16, -.56, .03]], .01, silk, { n: 16, sides: 6 }), knot = ball(.035, silk, { p: [.2, -.66, .03] });
  const tassel = strands(40, (rr) => V(.2 + (rr() - .5) * .02, -.67, .03), (rr, i, a) => V(.36 + (rr() - .5) * .18, -.98 + (rr() - .5) * .12, .012), .007, silk, 7);
  return item('Sabre', [part('blade', 'body', [0, .5, 0], [bladeM]), part('guard and bow', 'trim', [-.4, 0, .3], guard.concat([bow, pommel])), part('grip and tassel', 'detail', [.4, -.4, .3], grip.concat([loop, knot], tassel))], { yaw: -.75, fill: .94 });
}
