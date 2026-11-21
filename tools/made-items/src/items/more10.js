// Goods: a crate, a sack of grain, a vase, a rolled rug, a drum, a flute, dice, a sealed letter and a bottle of perfume.
import { THREE, V, M, part, item, turned, smooth, lathe, rod, ball, torus, cyl, rbox, slab, blob, sheet, many, aim, band, bar, strands, rng, drawn } from '../kit.js';
import { metal, wood, leather, cloth, matte, bone, paper, gem, glass, liquid, dark } from '../mats.js';

const TAU = Math.PI * 2, H2 = Math.PI / 2;
const sstep = (a, b, x) => { const t = Math.min(1, Math.max(0, (x - a) / (b - a))); return t * t * (3 - 2 * t); };
const fine = (m, n) => { for (const k of ['map', 'bumpMap']) if (m[k]) { m[k] = m[k].clone(); m[k].repeat.set(n, n); m[k].needsUpdate = true; } return m; };
const radiusAt = P => y => { for (let i = 0; i < P.length - 1; i++) { const a = P[i], b = P[i + 1]; if ((y - a[1]) * (y - b[1]) <= 0 && a[1] !== b[1]) return a[0] + (b[0] - a[0]) * (y - a[1]) / (b[1] - a[1]); } return P[P.length - 1][0]; };
const star = (x, y, s, n, k) => { const q = []; for (let i = 0; i < n * 2; i++) { const a = i / (n * 2) * TAU + H2, r = i % 2 ? s * k : s; q.push([x + Math.cos(a) * r, y + Math.sin(a) * r]); } return q; };

export function crate(c) {
  const wd = wood(c.body, { bump: 1 }), iron = metal(c.trim, { kind: 'forged', rough: .5 }), paint = matte(c.detail, { rough: .7 }), rope = matte('#b89a62', { rough: .95, bump: .9 }), r = rng(8);
  const W = 1.3, H = 1.0, D = 1.0, planks = [], frame = [], irons = [], nails = [];
  // Four boards to a side, a frame round each face and a brace across the front.
  const core = rbox(W - .06, H - .06, D - .06, .01, dark(), { p: [0, H / 2, 0] });
  for (let i = 0; i < 4; i++) { const y = .13 + i * .247, j = () => (r() - .5) * .012;
    for (const k of [-1, 1]) planks.push(rbox(W, .235, .04, .008, wd, { p: [j(), y, k * (D / 2 - .02)] }), rbox(.04, .235, D, .008, wd, { p: [k * (W / 2 - .02), y, j()] }));
    planks.push(rbox(W, .04, .235, .008, wd, { p: [j(), H - .02, -.37 + i * .247] })); }
  for (const k of [-1, 1]) {
    for (const s of [-1, 1]) frame.push(rbox(.11, H, .05, .01, wd, { p: [s * (W / 2 - .055), H / 2, k * (D / 2 + .02)] }), rbox(.05, H, .11, .01, wd, { p: [k * (W / 2 + .02), H / 2, s * (D / 2 - .055)] }));
    for (const y of [.055, H - .055]) frame.push(rbox(W - .22, .11, .05, .01, wd, { p: [0, y, k * (D / 2 + .02)] }), rbox(.05, .11, D - .22, .01, wd, { p: [k * (W / 2 + .02), y, 0] }));
  }
  const brace = bar([[-.52, .13], [.52, .87]], .11, .05, wd, { p: [0, 0, D / 2 + .02] });
  for (const sx of [-1, 1]) for (const sy of [0, 1]) for (const sz of [-1, 1]) {
    const x = sx * (W / 2 - .06), y = sy ? H - .08 : .08, z = sz * (D / 2 + .048);
    irons.push(rbox(.2, .16, .014, .004, iron, { p: [x - sx * .03, y, z] }), rbox(.014, .16, .2, .004, iron, { p: [sx * (W / 2 + .048), y, sz * (D / 2 - .09)] }));
    for (const dx of [-.07, .03]) nails.push({ p: V(x + sx * dx, y, z + sz * .008), s: [1, 1, .5] });
  }
  const nailsM = many(new THREE.SphereGeometry(.018, 10, 8), iron, nails);
  // The maker's mark is stencilled on the front, and a rope handle hangs at each end.
  const mark = [slab(star(-.02, .5, .2, 4, .42), .004, 0, paint, { p: [-.26, 0, D / 2 + .047] }), rbox(.3, .035, .004, .001, paint, { p: [.3, .64, D / 2 + .047] }), rbox(.3, .035, .004, .001, paint, { p: [.3, .56, D / 2 + .047] })];
  const grips = [-1, 1].map(k => rod([[k * (W / 2 + .04), .7, -.2], [k * (W / 2 + .1), .62, -.14], [k * (W / 2 + .17), .46, 0], [k * (W / 2 + .1), .62, .14], [k * (W / 2 + .04), .7, .2]], .028, rope, { n: 24, sides: 8 }));
  const knots = [-1, 1].map(k => [-1, 1].map(s => ball(.045, rope, { p: [k * (W / 2 + .05), .7, s * .2] }))).flat();
  return item('Crate', [part('boards', 'body', [0, 0, 0], [core, planks, frame, brace]), part('ironwork', 'trim', [0, .4, .4], irons.concat([nailsM])), part('mark', 'detail', [0, 0, .9], mark), part('rope handles', '', [.6, 0, 0], grips.concat(knots))], { stand: true, yaw: .55, el: 22 });
}

export function sack(c) {
  const hemp = fine(cloth(c.body, { bump: 1.3, sheen: .15 }), 3), cord = matte(c.trim, { rough: .95, bump: .9 }), grainM = matte(c.detail, { rough: .5, wet: .2 }), r = rng(21);
  // A full sack slumps: flat where it sits, widest low down, creased where the tie pulls it in.
  const body = blob([.68, .78, .58], v => { const a = Math.atan2(v.z, v.x), low = v.y < -.72 ? .72 / -v.y : 1; return low * (1 + .16 * sstep(.3, -.6, v.y) - .34 * sstep(.45, 1, v.y) + .035 * Math.sin(a * 7 + v.y * 4) * sstep(-.2, .8, v.y) + .02 * Math.sin(a * 3 + 1)); }, hemp, { p: [0, .56, 0], seg: 80, seg2: 56 });
  const ng = lathe(smooth([[.2, 1.02], [.13, 1.14], [.12, 1.22], [.24, 1.36], [.33, 1.44]], 24), 72), np = ng.attributes.position;
  for (let i = 0; i < np.count; i++) { const x = np.getX(i), y = np.getY(i), z = np.getZ(i), k = 1 + .16 * Math.sin(Math.atan2(z, x) * 9) * sstep(1.02, 1.4, y) + .05 * Math.sin(Math.atan2(z, x) * 4 + 1); np.setXYZ(i, x * k, y + .03 * Math.sin(Math.atan2(z, x) * 5) * sstep(1.2, 1.44, y), z * k); }
  ng.computeVertexNormals(); const neck = M(ng, hemp), inside = ball(.16, dark(), { s: [1, .3, 1], p: [0, 1.33, 0] });
  const tie = [torus(.14, .022, cord, { r: [H2, 0, 0], p: [0, 1.17, 0] }), torus(.14, .022, cord, { r: [H2, 0, .12], p: [0, 1.21, 0] }), ball(.04, cord, { p: [.02, 1.19, .15] }), rod([[.02, 1.19, .15], [.12, 1.08, .3], [.16, .86, .42]], .016, cord, { n: 14, sides: 6 }), rod([[.02, 1.19, .15], [-.1, 1.1, .28], [-.2, .94, .4]], .016, cord, { n: 14, sides: 6 })];
  // Some of the grain lies in a heap at its foot.
  const heap = blob([.44, .1, .34], v => 1 + .1 * Math.sin(v.x * 8) * Math.sin(v.z * 6), grainM, { p: [.5, .0, .62] }), grains = [];
  for (let i = 0; i < 150; i++) { const a = r() * TAU, d = Math.sqrt(r()) * 1.25, x = .5 + Math.cos(a) * d * .46, z = .62 + Math.sin(a) * d * .36, y = d < 1 ? .1 * Math.sqrt(Math.max(0, 1 - d * d)) + .008 : .008; grains.push({ p: V(x, y, z), r: [r() * 3, r() * 3, r() * 3], s: [1, .55, 1.9] }); }
  const grainsM = many(new THREE.SphereGeometry(.02, 8, 6), grainM, grains);
  return item('Grain sack', [part('sack', 'body', [0, 0, 0], [body, neck, inside]), part('tie', 'trim', [0, .6, .3], tie), part('grain', 'detail', [.5, 0, .5], [heap, grainsM])], { stand: true, yaw: .2, el: 16 });
}

export function vase(c) {
  const glaze = matte(c.body, { rough: .26, wet: .8, bump: .1 }), slip = matte(c.trim, { rough: .34, wet: .6 }), paint = matte(c.detail, { rough: .3, wet: .7 });
  const P = smooth([[0, .04], [.2, .04], [.26, 0], [.28, .05], [.22, .14], [.3, .4], [.5, .9], [.52, 1.2], [.4, 1.5], [.2, 1.72], [.16, 1.9], [.24, 2.06], [.27, 2.1]], 80), R = radiusAt(P.slice(14));
  const pot = turned(P, glaze, { sm: 0 }), inner = cyl(.1, .2, .2, dark(), { p: [0, 2.0, 0] });
  const lip = band(.25, 2.09, .05, .03, slip), foot = turned([[0, 0], [.27, 0], [.3, .04], [.29, .08], [.22, .14]], slip, { sm: 20 });
  const handles = [-1, 1].map(k => rod([[k * .17, 1.92, 0], [k * .34, 1.98, 0], [k * .5, 1.82, 0], [k * .52, 1.56, 0], [k * .44, 1.42, 0]], s => .04 + .012 * Math.sin(s * Math.PI), slip, { n: 30, sides: 12 }));
  // Painted round the belly: a running zigzag between two lines, and a row of dots on the shoulder.
  const zz = []; for (let i = 0; i < 40; i++) { const a = i / 40 * TAU, y = i % 2 ? 1.16 : .98, rr = R(y) + .004; zz.push(V(Math.cos(a) * rr, y, Math.sin(a) * rr)); }
  const zig = rod(zz, .014, paint, { sides: 6, closed: true }), lines = [.9, 1.24, 1.72].map(y => band(R(y) - .006, y, .028, .011, paint));
  const dots = many(new THREE.SphereGeometry(1, 12, 8), paint, Array.from({ length: 14 }, (_, i) => { const a = i / 14 * TAU, y = 1.42, rr = R(y) + .002; return { p: V(Math.cos(a) * rr, y, Math.sin(a) * rr), q: aim(V(Math.cos(a), .6, Math.sin(a))), s: [.04, .01, .04] }; }));
  return item('Vase', [part('vase', 'body', [0, 0, 0], [pot, inner]), part('lip, foot and handles', 'trim', [0, .6, 0], [lip, foot].concat(handles)), part('painting', 'detail', [0, 0, .8], [zig, dots].concat(lines))], { stand: true, yaw: .2, el: 12 });
}

export function rug(c) {
  const weave = drawn(256, 256, (x, w, h) => { x.strokeStyle = 'rgba(255,255,255,.5)'; x.lineWidth = 3; for (let i = 0; i < 12; i++) for (let j = 0; j < 4; j++) { const cx = (i + .5) * w / 12, cy = (j + .5) * h / 4; x.beginPath(); x.moveTo(cx, cy - 22); x.lineTo(cx + 9, cy); x.lineTo(cx, cy + 22); x.lineTo(cx - 9, cy); x.closePath(); x.stroke(); } }, [.62, .62, .62]);
  weave.wrapS = weave.wrapT = THREE.RepeatWrapping;
  const pile = cloth(c.body, { bump: .9, sheen: .5 }); pile.map = weave;
  const edge = cloth(c.trim, { bump: .9, sheen: .5 }), tieM = matte(c.detail, { rough: .9, bump: .8 }), r = rng(14), Wd = 1.9, T = 3.4 * TAU, R1 = .31, YC = R1 + .012;
  // The rug as one strip: wound up from its middle, the last arm's length lying open on the floor.
  const strip = (x0, x1, lift) => (u, v) => {
    const x = (x0 + (x1 - x0) * u - .5) * Wd;
    if (v < .8) { const al = (v / .8 - 1) * T, rr = .06 + (R1 - .06) * (1 + al / T) + lift; return V(x, YC - rr * Math.cos(al), rr * Math.sin(al)); }
    return V(x, .012 + lift + .006 * Math.sin(x * 9 + v * 30), (v - .8) / .2 * .8);
  };
  const body = sheet(strip(0, 1, 0), 24, 160, pile, { uvx: 1, uvy: 6 });
  const borders = [[.03, .09], [.91, .97]].map(([a, b]) => sheet(strip(a, b, .005), 2, 160, edge)), lines = [[.13, .145], [.855, .87]].map(([a, b]) => sheet(strip(a, b, .005), 1, 160, edge));
  const cross = sheet((u, v) => V((.03 + .94 * u - .5) * Wd, .018 + .006 * Math.sin((.03 + .94 * u - .5) * Wd * 9 + 28), .7 + v * .06), 20, 1, edge);
  const medal = slab([[0, .22], [.3, 0], [0, -.22], [-.3, 0]], .004, 0, edge, { r: [-H2, 0, 0], p: [0, .022, .38] });
  const ends = [-1, 1].map(k => { const pts = []; for (let i = 0; i <= 120; i++) { const al = (i / 120 - 1) * T, rr = .06 + (R1 - .06) * (1 + al / T); pts.push(V(k * Wd / 2, YC - rr * Math.cos(al), rr * Math.sin(al))); } return rod(pts, .011, pile, { sides: 6 }); });
  const fringe = many(new THREE.CylinderGeometry(.006, .004, .13, 5), tieM, Array.from({ length: 90 }, (_, i) => ({ p: V((i / 89 - .5) * Wd * .98, .012, .8 + .06), r: [H2 + (r() - .5) * .2, 0, (r() - .5) * .4] })));
  const ties = [-.5, .5].map(x => [torus(R1 + .014, .02, tieM, { r: [0, H2, 0], p: [x, YC, 0] }), ball(.04, tieM, { p: [x, YC + .1, R1 + .01] }), rod([[x, YC + .1, R1 + .01], [x + .05, YC - .05, R1 + .08], [x + .06, .1, R1 + .12]], .014, tieM, { n: 10, sides: 6 }), rod([[x, YC + .1, R1 + .01], [x - .06, YC - .02, R1 + .1], [x - .09, .14, R1 + .1]], .014, tieM, { n: 10, sides: 6 })]).flat();
  return item('Rolled rug', [part('rug', 'body', [0, 0, 0], [body].concat(ends)), part('border', 'trim', [0, .2, .5], borders.concat(lines, [cross, medal])), part('ties and fringe', 'detail', [0, .5, .3], ties.concat([fringe]))], { stand: true, yaw: .5, el: 24 });
}

export function drum(c) {
  const shell = wood(c.body, { polish: .4 }), hoopM = leather(c.trim, { bump: .8 }), hide = fine(leather(c.detail, { bump: .5 }), 2), rope = matte('#c8b48a', { rough: .95, bump: .9 }), stick = wood('#8a6238', { polish: .3 });
  const P = smooth([[.58, .08], [.64, .5], [.58, .92]], 24), barrel = turned(P, shell, { sm: 0 });
  const heads = [cyl(.6, .6, .03, hide, { p: [0, .935, 0], seg: 48 }), cyl(.6, .6, .03, hide, { p: [0, .065, 0], seg: 48 })], hoops = [.94, .06].map(y => torus(.61, .034, hoopM, { seg: 64, r: [H2, 0, 0], p: [0, y, 0] }));
  // The heads are pulled tight by one rope laced up and down between the hoops.
  const N = 22, lace = []; for (let i = 0; i < N; i++) { const a = i / N * TAU, b = (i + .5) / N * TAU; lace.push(V(Math.cos(a) * .637, i % 2 ? .1 : .9, Math.sin(a) * .637), V(Math.cos(b) * .66, .5, Math.sin(b) * .66)); }
  const lacing = rod(lace, .013, rope, { sides: 6, closed: true });
  const tugs = Array.from({ length: N / 2 }, (_, i) => { const a = (i * 2 + .5) / N * TAU; return torus(.035, .012, hoopM, { r: [0, -a, 0], p: [Math.cos(a) * .655, .5, Math.sin(a) * .655] }); });
  // The beater lies in front of it.
  const beater = [rod([[.55, .03, 1.02], [-.5, .05, .9]], s => .028 - .006 * s, stick, { sides: 10 }), ball(.085, hide, { p: [-.56, .085, .893], s: [1.1, .95, .95] })];
  return item('Drum', [part('shell', 'body', [0, 0, 0], [barrel]), part('hoops', 'trim', [0, .5, 0], hoops.concat(tugs)), part('heads', 'detail', [0, .8, 0], heads.concat([beater[1]])), part('lacing and stick', '', [0, 0, .6], [lacing, beater[0]])], { stand: true, yaw: .2, el: 22 });
}

export function flute(c) {
  const wd = wood(c.body, { polish: .7, bump: .3 }), br = metal(c.trim, { kind: 'cast', rough: .26 }), silk = cloth(c.detail, { sheen: 1 });
  // Turned in three joints; the holes are on the side that faces out.
  const tubeM = turned([[.0, -1.6], [.07, -1.6], [.075, -1.5], [.068, -.5], [.074, -.44], [.074, .4], [.08, .46], [.082, 1.3], [.09, 1.4], [.085, 1.55], [.06, 1.6], [0, 1.6]], wd, { sm: 0, seg: 28 });
  const rings = [[-1.52, .075], [-.47, .074], [.43, .08], [1.38, .089]].map(([y, rr]) => band(rr, y, .07, .012, br, { seg: 28 }));
  const holes = [-1.2, -.98, -.76, -.2, .02, .24].map(y => cyl(.024, .024, .02, dark(), { r: [H2, 0, 0], p: [0, y, .066], seg: 16 })).concat([cyl(.034, .034, .02, dark(), { r: [H2, 0, 0], p: [0, 1.08, .086], seg: 16, s: [1, 1, 1.3] }), cyl(.045, .045, .01, dark(), { p: [0, -1.603, 0] })]);
  const plate = band(.084, 1.08, .16, .008, br, { seg: 28 });
  const cordP = [[.0, .9, .09], [.12, .86, .1], [.2, .7, .08], [.22, .5, .06]], cord = [torus(.086, .014, silk, { r: [H2, 0, 0], p: [0, .9, 0] }), rod(cordP, .012, silk, { n: 12, sides: 6 }), turned([[0, .5], [.03, .49], [.05, .44], [.045, .38], [.03, .35]], silk, { sm: 16, p: [.22, 0, .06] })];
  const tassel = strands(30, rr => V(.22 + (rr() - .5) * .05, .36, .06 + (rr() - .5) * .05), (rr, i, a) => V(a.x + (rr() - .5) * .1, .0 - rr() * .06, a.z + (rr() - .5) * .1), .009, silk, 3);
  return item('Flute', [part('flute', 'body', [0, 0, 0], [tubeM].concat(holes)), part('rings', 'trim', [0, 0, .5], rings.concat([plate])), part('cord and tassel', 'detail', [.4, 0, .2], cord.concat(tassel))], { yaw: -.35, fill: .97 });
}

export function dice(c) {
  const ivory = bone(c.body, { gloss: .5 }), pipM = matte(c.trim, { rough: .5 }), stone = gem(c.detail, { inner: 1.4, lit: .2, edges: .6 }), pips = [], S = .5;
  const L = { 1: [[0, 0]], 2: [[-1, -1], [1, 1]], 3: [[-1, -1], [0, 0], [1, 1]], 4: [[-1, -1], [1, -1], [-1, 1], [1, 1]], 5: [[-1, -1], [1, -1], [0, 0], [-1, 1], [1, 1]], 6: [[-1, -1], [1, -1], [-1, 0], [1, 0], [-1, 1], [1, 1]] };
  // Opposite faces of a die add to seven.
  const faces = [[V(0, 1, 0), V(1, 0, 0), V(0, 0, 1)], [V(0, -1, 0), V(1, 0, 0), V(0, 0, 1)], [V(0, 0, 1), V(1, 0, 0), V(0, 1, 0)], [V(0, 0, -1), V(1, 0, 0), V(0, 1, 0)], [V(1, 0, 0), V(0, 0, 1), V(0, 1, 0)], [V(-1, 0, 0), V(0, 0, 1), V(0, 1, 0)]];
  const cubes = [[[-.5, S / 2, .1], .4, [5, 2, 6, 1, 3, 4]], [[.22, S / 2, .52], -.3, [1, 6, 4, 3, 2, 5]]].map(([p, yaw, nums]) => {
    const cs = Math.cos(yaw), sn = Math.sin(yaw), turn = v => V(v.x * cs + v.z * sn, v.y, -v.x * sn + v.z * cs);
    faces.forEach(([n, u, w], f) => { for (const [a, b] of L[nums[f]]) { const q = n.clone().multiplyScalar(S / 2 - .004).addScaledVector(u, a * .13).addScaledVector(w, b * .13); pips.push({ p: turn(q).add(V(p[0], p[1], p[2])), q: aim(turn(n)), s: [.048, .014, .048] }); } });
    return rbox(S, S, S, .07, ivory, { r: [0, yaw, 0], p: p });
  });
  const pipsM = many(new THREE.SphereGeometry(1, 14, 10), pipM, pips);
  // The twenty-sided one is cut from a stone and rests on a face.
  const ig = new THREE.IcosahedronGeometry(.42, 0), ip = ig.attributes.position, a = new THREE.Vector3().fromBufferAttribute(ip, 0), b = new THREE.Vector3().fromBufferAttribute(ip, 1), d = new THREE.Vector3().fromBufferAttribute(ip, 2), n = b.clone().sub(a).cross(d.clone().sub(a)).normalize();
  ig.applyQuaternion(new THREE.Quaternion().setFromUnitVectors(n, V(0, -1, 0))); ig.computeBoundingBox(); ig.rotateY(.5); ig.translate(.42, -ig.boundingBox.min.y, -.34); ig.computeVertexNormals();
  const d20 = M(ig, stone);
  return item('Dice', [part('dice', 'body', [0, 0, 0], cubes), part('pips', 'trim', [0, .5, .3], [pipsM]), part('stone die', 'detail', [.5, .3, -.3], [d20])], { stand: true, yaw: .15, el: 26, lights: [{ p: [.42, .4, -.34], color: c.detail, power: .25 }] });
}

export function letter(c) {
  const plain = paper(c.body), ribbon = cloth(c.trim, { sheen: 1 }), waxM = matte(c.detail, { rough: .32, wet: .6 }), W = 1.7, H = 1.16;
  // A sheet folded in on itself: the back, and four flaps that meet under the seal.
  const bend = (x, y) => .03 * Math.sin(x * 2.2 + .4) + .02 * Math.cos(y * 2.6);
  const back = sheet((u, v) => { const x = (u - .5) * W, y = (v - .5) * H; return V(x, y, bend(x, y) - .012); }, 16, 12, plain);
  const flap = (pts, lift) => sheet((u, v) => { const a = pts[0], b = pts[1], t = pts[2], x = (a[0] + (b[0] - a[0]) * u) * (1 - v) + t[0] * v, y = (a[1] + (b[1] - a[1]) * u) * (1 - v) + t[1] * v; return V(x, y, bend(x, y) + lift * (.4 + .6 * v)); }, 10, 8, plain);
  const w2 = W / 2, h2 = H / 2, flaps = [flap([[-w2, -h2], [-w2, h2], [-.06, 0]], .006), flap([[w2, h2], [w2, -h2], [.06, 0]], .006), flap([[w2, -h2], [-w2, -h2], [0, -.05]], .012), flap([[-w2, h2], [w2, h2], [0, -.14]], .02)];
  // A ribbon runs round it both ways and is caught under the wax.
  const z0 = bend(0, -.12) + .03, strips = [sheet((u, v) => { const x = (u - .5) * (W + .02), y = -.12 + (v - .5) * .11; return V(x, y, bend(x, y) + .026); }, 16, 1, ribbon), sheet((u, v) => { const x = (v - .5) * .11, y = (u - .5) * (H + .02); return V(x, y, bend(x, y) + .029); }, 12, 1, ribbon)];
  const tails = [-1, 1].map(k => sheet((u, v) => V(k * (.04 + v * .16) + (u - .5) * .1, -.2 - v * .52, z0 + .02 * Math.sin(v * 6 + k)), 1, 12, ribbon));
  const seal = [blob([.2, .19, .04], v => 1 + .09 * Math.sin(Math.atan2(v.y, v.x) * 5 + 1) + .05 * Math.sin(Math.atan2(v.y, v.x) * 9), waxM, { p: [0, -.12, z0 + .01] }), torus(.115, .014, waxM, { p: [0, -.12, z0 + .045] }), slab(star(0, -.12, .085, 5, .45), .02, .006, waxM, { p: [0, 0, z0 + .048] })];
  return item('Sealed letter', [part('letter', 'body', [0, 0, 0], [back].concat(flaps)), part('ribbon', 'trim', [0, -.3, .4], strips.concat(tails)), part('seal', 'detail', [0, 0, .9], seal)], { fill: .95 });
}

export function perfume(c) {
  const gl = glass(c.body, { tint: .34 }), gold = metal(c.trim, { kind: 'cast', rough: .24 }), scent = liquid(c.detail, { lit: .45, inner: 1.3 }), cut = gem(c.body, { edges: .5, inner: .5, pale: .5 }), silk = cloth(c.detail, { sheen: 1 });
  // A round flask pressed flat, cut in facets, on a little gold foot.
  const flaskP = [[0, .2], [.2, .2], [.42, .34], [.52, .62], [.46, .9], [.26, 1.08], [.1, 1.14], [.085, 1.34], [.11, 1.36]], flask = turned(flaskP, gl, { sm: 0, seg: 10, s: [1, 1, .6] });
  const fill = turned([[0, .23], [.19, .23], [.4, .36], [.49, .62], [.46, .8], [0, .8]], scent, { sm: 0, seg: 10, s: [1, 1, .56] });
  const foot = turned([[0, 0], [.26, 0], [.28, .04], [.16, .1], [.12, .2], [.2, .22]], gold, { sm: 30, s: [1, 1, .7] }), collar = [band(.088, 1.16, .05, .016, gold), band(.11, 1.35, .05, .014, gold)];
  const stopper = turned([[0, 1.36], [.07, 1.38], [.05, 1.46], [.15, 1.66], [.11, 1.86], [0, 2.0]], cut, { sm: 0, seg: 6 }), plug = cyl(.06, .05, .12, gold, { p: [0, 1.32, 0] });
  const vines = [-1, 1].map(k => rod([[k * .2, .22, .19], [k * .34, .46, .23], [k * .22, .66, .3], [k * .3, .86, .2], [k * .14, 1.04, .17]], s => .016 - .008 * s, gold, { n: 30, sides: 8 }));
  const tie = [torus(.1, .014, silk, { r: [H2, 0, 0], p: [0, 1.24, 0] }), rod([[.1, 1.24, .02], [.3, 1.16, .1], [.44, .96, .12]], .011, silk, { n: 12, sides: 6 }), turned([[0, .96], [.03, .95], [.045, .9], [.03, .84]], silk, { sm: 12, p: [.44, 0, .12] })];
  const tassel = strands(24, rr => V(.44 + (rr() - .5) * .04, .85, .12 + (rr() - .5) * .04), (rr, i, a) => V(a.x + (rr() - .5) * .09, .56 - rr() * .05, a.z + (rr() - .5) * .09), .008, silk, 6);
  return item('Perfume', [part('flask and stopper', 'body', [0, 0, 0], [flask, stopper]), part('gold', 'trim', [0, 0, .5], [foot, plug].concat(collar, vines)), part('scent and tassel', 'detail', [0, 0, .9], [fill].concat(tie, tassel))], { stand: true, yaw: .25, el: 12, lights: [{ p: [0, .6, 0], color: c.detail, power: .5 }] });
}
