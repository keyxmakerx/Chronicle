// More things of magical origin: horn, fallen star, ink, shell, root, egg and wing.
import { THREE, V, M, X, Y, Z, part, item, turned, rod, ball, torus, cyl, blob, sheet, fan, rim, many, aim, crystalGeo, loft, ring, tube, path, drawn, makeNoise, fbm, rng } from '../kit.js';
import { metal, wood, leather, rock, paper, matte, gem, glass, liquid, glow, ghost, shell } from '../mats.js';

const C = hex => new THREE.Color(hex), ss = x => { x = Math.max(0, Math.min(1, x)); return x * x * (3 - 2 * x); };
// Light laid over a solid as an added layer, bright where fn(vertex index, geometry) says.
function lit(mesh, hex, fn) {
  const g = mesh.geometry.clone(), n = g.attributes.position.count, col = new Float32Array(n * 3), c = C(hex);
  for (let i = 0; i < n; i++) { const f = fn(i, g); col[i * 3] = c.r * f; col[i * 3 + 1] = c.g * f; col[i * 3 + 2] = c.b * f; }
  g.setAttribute('color', new THREE.BufferAttribute(col, 3));
  const m = new THREE.Mesh(g, new THREE.MeshBasicMaterial({ vertexColors: true, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false })); m.userData.glow = true; m.userData.nofit = true; m.userData.noshadow = true; m.renderOrder = 1; return m;
}

export function unicorn(c, t) {
  const L = 2.7, turns = 3.4, tt = t || 0, ph = tt * Math.PI * 2, pearl = shell('#' + C(c.body).multiplyScalar(.78).getHexString(), { metal: .05, rough: .3, irid: .45, bump: .3 }), silver = metal(c.trim, { kind: 'steel', rough: .42 }), nA = 42, N = 240;
  const cen = s => V(.05 * Math.sin(s * 2.6), -L / 2 + s * L, 0), R = s => .125 * Math.pow(1 - s, .8) + .004, rings = [];
  // Three strands laid up like rope round a straight taper, the grooves fading toward the point.
  for (let i = 0; i <= N; i++) { const s = i / N, pts = []; for (let j = 0; j < nA; j++) { const th = j / nA * Math.PI * 2, w = 3 * (th - turns * 2 * Math.PI * s), k = R(s) * (1 + (.075 * Math.cos(w) - .02 * Math.cos(2 * w)) * (1 - s * .5)); pts.push([Math.cos(th) * k, -Math.sin(th) * k]); } rings.push(ring(cen(s), X, Z, pts)); }
  const horn = M(loft(rings), pearl);
  { const uv = horn.geometry.attributes.uv; for (let i = 0; i < uv.count; i++) uv.setXY(i, uv.getX(i) * 2, uv.getY(i) * 5); }
  const run = lit(horn, c.detail, i => { const s = Math.floor(i / (nA + 1)) / N; let d = Math.abs(s - tt); d = Math.min(d, 1 - d); return .42 * Math.exp(-d * d / .012) + .02; });
  const y0 = -L / 2, cut = cyl(.128, .128, .01, matte('#b8a888', { rough: .6, bump: .6 }), { p: [0, y0 - .004, 0] }), pith = cyl(.045, .045, .012, matte('#6a5642', { rough: .7 }), { p: [0, y0 - .005, 0] });
  const collar = turned([[.13, y0 + .1], [.15, y0 + .105], [.156, y0 + .16], [.15, y0 + .27], [.134, y0 + .29], [.12, y0 + .28]], silver, { sm: 30, seg: 56 }), beads = [.12, .25].map(d => torus(.152, .011, silver, { r: [Math.PI / 2, 0, 0], p: [0, y0 + d, 0] }));
  const lug = torus(.045, .013, silver, { r: [0, Math.PI / 2, 0], p: [.185, y0 + .19, 0] }), thong = rod([[.21, y0 + .19, 0], [.34, y0 + .05, 0], [.3, y0 - .22, 0], [.5, y0 - .36, 0], [.74, y0 - .24, 0], [.8, y0 - .02, 0]], .018, leather('#5a3a2a'), { n: 30, sides: 8 });
  const glints = []; for (let k = 1; k < 9; k++) { const s = k / 9.4, q = cen(s), z = R(s) * 1.08 + .012, b = Math.pow(Math.max(0, Math.sin(ph + k * 1.9)), 5), len = .015 + .07 * b, gm = glow(c.detail, .8 + 3 * b, { add: true }); if (b < .02) continue; glints.push(ball(1, gm, { s: [len, .006, .006], p: [q.x, q.y, z], seg: 10, seg2: 6 }), ball(1, gm, { s: [.006, len, .006], p: [q.x, q.y, z], seg: 10, seg2: 6 }), ball(.01 + .01 * b, gm, { p: [q.x, q.y, z], seg: 8, seg2: 6 })); }
  [horn, run, cut, pith, collar, lug, thong].concat(beads, glints).forEach(m => m.geometry.rotateX(-.055));
  const lp = cen(tt);
  return item('Unicorn horn', [part('horn', 'body', [0, .25, 0], [horn, cut, pith]), part('mount', 'trim', [0, -.35, 0], [collar, lug, thong].concat(beads)), part('the light that runs up it', 'detail', [0, 0, .7], [run].concat(glints))], { yaw: -.62, fill: .95, lights: [{ p: [lp.x, lp.y, .35], color: c.detail, power: .3 }], halo: .8 });
}

export function starfrag(c, t) {
  const n = makeNoise(19, 32), r = rng(12), tt = t || 0, ph = tt * Math.PI * 2, pw = 1 + .4 * Math.sin(ph), S = [.5, .95, .46];
  const pits = Array.from({ length: 46 }, () => { const z = r() * 2 - 1, a = r() * 6.283, q = Math.sqrt(1 - z * z); return V(Math.cos(a) * q, z, Math.sin(a) * q); });
  // Sky-iron: melted smooth on the way down and pressed all over with thumbprints.
  const cuts = [[V(.5, .7, .5).normalize(), .62], [V(-.7, .5, .3).normalize(), .66], [V(.2, -.3, .95).normalize(), .7], [V(.8, -.2, -.5).normalize(), .68]], k = v => { let d = 0; for (const q of pits) { const a = 1 - v.dot(q); if (a < .08) d += Math.exp(-a / .012); } return 1 + (fbm(n, v.x * 1.7 + 4, v.y * 1.4 + v.z * 1.9 + 2, 3) - .5) * .55 + (fbm(n, v.x * 5 + 1, v.y * 5 + v.z * 4, 2) - .5) * .2 - Math.min(1.2, d) * .055; }, kc = v => { let q = k(v); for (const [nn, cc] of cuts) { const a = v.dot(nn); if (a * q > cc) q = cc / a; } return q; };
  const body = blob(S, kc, metal(c.body, { kind: 'forged', rough: .42, bump: .7 }), { seg: 140, seg2: 96 });
  const gm = gem(c.detail, { vc: true, lit: .75 * pw, inner: .8 }), spots = [], grown = [];
  for (let i = 0; i < 6; i++) { const d = V(r() * 2 - 1, r() * 1.6 - .5, r() * 1.5 - .35).normalize(), q = kc(d) * .88, pos = V(d.x * q * S[0], d.y * q * S[1], d.z * q * S[2]), len = .1 + r() * .13, m = M(crystalGeo(.024 + r() * .02, len, len * .45, { seed: i + 30, hex: c.detail }), gm); m.geometry.applyQuaternion(aim(d.clone().add(V((r() - .5) * .7, (r() - .5) * .7, (r() - .5) * .7)))); m.geometry.translate(pos.x, pos.y, pos.z); grown.push(m); spots.push(pos); }
  const p0 = body.geometry.attributes.position, vv = new THREE.Vector3(), warm = lit(body, c.detail, i => { vv.fromBufferAttribute(p0, i); let f = 0; for (const q of spots) f += Math.exp(-vv.distanceTo(q) / .05); const d = vv.clone().normalize(), vein = Math.pow(1 - Math.abs(2 * fbm(n, d.x * 3 + 9, d.y * 3 + d.z * 3.3, 3) - 1), 22); return Math.min(1, f * .3 * pw + vein * 1.1 * pw); });
  const all = [body, warm].concat(grown); let lo = 9; all.forEach(m => { m.geometry.rotateZ(.42); m.geometry.rotateX(.12); }); body.geometry.computeBoundingBox(); lo = body.geometry.boundingBox.min.y; all.forEach(m => m.geometry.translate(0, .05 - lo, 0));
  const earth = rock(c.trim, { bump: 2.6, rough: .95 }), crater = turned([[0, .1], [.3, .13], [.5, .22], [.62, .27], [.74, .2], [.95, .07], [1.1, .02], [1.22, 0]], earth, { sm: 60, seg: 96 });
  { const q = crater.geometry.attributes.position; for (let i = 0; i < q.count; i++) { const x = q.getX(i), y = q.getY(i), z = q.getZ(i), rr = Math.hypot(x, z), kk = 1 + (fbm(n, x * 3 + 7, z * 3, 3) - .5) * .25; q.setXYZ(i, x * kk, Math.max(0, y * (1 + (fbm(n, x * 5, z * 5 + 3, 3) - .5) * 1.3 * Math.min(1, rr * 3))), z * kk); } crater.geometry.computeVertexNormals(); }
  const clods = Array.from({ length: 14 }, () => { const a = r() * 6.28, d = .8 + r() * .55, s = .03 + r() * .06; return M(new THREE.DodecahedronGeometry(s, 0), rock(c.trim, { flat: true, bump: 1 }), { r: [r() * 3, r() * 3, 0], p: [Math.cos(a) * d, s * .7, Math.sin(a) * d] }); });
  const motes = Array.from({ length: 16 }, () => { const a = r() * 6.28, d = .15 + r() * .55, h = (r() + tt) % 1, m = ball(.01 + r() * .012, glow(c.detail, 2.6 * (1 - h) * Math.min(1, h * 6), { add: true }), { p: [Math.cos(a) * d, .5 + h * 1.3, Math.sin(a) * d], seg: 8, seg2: 6 }); m.userData.nofit = true; return m; });
  return item('Star fragment', [part('sky-iron', 'body', [0, .2, 0], [body]), part('where it landed', 'trim', [0, -.4, 0], [crater].concat(clods)), part('starlight still in it', 'detail', [0, .3, .8], grown.concat([warm], motes))], { stand: true, yaw: .3, el: 16, lights: [{ p: [.2, 1.0, .6], color: c.detail, power: 1.1 * pw }], halo: .8 });
}

export function ink(c, t) {
  const gl = glass(c.body, { tint: .15 }), ck = wood(c.trim, { bump: .4 }), cord = matte('#6a5a44', { rough: .9, bump: .8 }), tt = t || 0, ph = tt * Math.PI * 2, r = rng(15), lv = .64;
  const hx = '#' + C(c.detail).getHexString(), lite = '#' + C(c.detail).lerp(C('#ffffff'), .12).getHexString(), wet = liquid(hx, { lit: .035, inner: .5 }), film = liquid(lite, { lit: .08, inner: 1.3 });
  const body = turned([[0, .05], [.2, .025], [.4, 0], [.47, .04], [.5, .16], [.47, .5], [.35, .82], [.2, 1.02], [.135, 1.14], [.125, 1.42], [.15, 1.46], [.19, 1.5], [.19, 1.54], [.13, 1.55]], gl, { sm: 110, seg: 64 });
  const fill = turned([[0, .08], [.38, .06], [.45, .1], [.47, .18], [.445, .5], [.41, lv], [0, lv]], wet, { sm: 0, seg: 48 });
  // It boils without heat: each bubble swells, bursts and leaves a ring that spreads and is gone.
  const bub = [];
  for (let i = 0; i < 12; i++) {
    const a = r() * 6.28, d = Math.sqrt(r()) * .3, size = .04 + r() * .075, u = (tt + r()) % 1, k = u < .82 ? Math.pow(u / .82, .55) : 0, x = Math.cos(a) * d, z = Math.sin(a) * d;
    if (k > .05) bub.push(ball(size * k, film, { p: [x, lv + size * k * .3, z], seg: 28, seg2: 20 }));
    else if (u >= .82) { const w = (u - .82) / .18; bub.push(torus(size * (.8 + w * .8), .008 * (1 - w) + .001, film, { r: [Math.PI / 2, 0, 0], p: [x, lv + .008, z], seg: 20 })); }
  }
  for (let i = 0; i < 16; i++) { const a = i / 16 * 6.28 + r(), s = .014 + r() * .02; bub.push(ball(s, film, { p: [Math.cos(a) * (.4 - s), lv + s * .4, Math.sin(a) * (.4 - s)], seg: 10, seg2: 8 })); }
  const vap = [0, 1, 2].map(i => { const pts = []; for (let j = 0; j <= 26; j++) { const s = j / 26; pts.push(V(Math.sin(s * 5 + i * 2.1 - ph) * .1 * s + (i - 1) * .03, 1.5 + s * .6, Math.cos(s * 4 + i - ph) * .08 * s)); } return rod(pts, s => .01 + .05 * s * (1 - s * .4), ghost(lite, .2), { sides: 8 }); });
  const ca = .95, run = rod([[.19, 1.5], [.152, 1.445], [.128, 1.3], [.136, 1.15], [.2, 1.03], [.29, .91]].map(q => [Math.sin(ca) * (q[0] + .004), q[1], Math.cos(ca) * (q[0] + .004)]), s => .012 + .006 * s, wet, { n: 30, sides: 8 }), bead = ball(.03, wet, { s: [1, 1.35, 1], p: [Math.sin(ca) * .3, .9, Math.cos(ca) * .3] });
  const blot = blob([.17, .008, .12], v => 1 + (Math.sin(Math.atan2(v.z, v.x) * 5) * .12), wet, { p: [.62, .008, .56] });
  const cork = cyl(.105, .125, .2, ck), stain = cyl(.1, .1, .012, matte('#' + C(c.detail).multiplyScalar(.4).getHexString(), { rough: .4, wet: .5 }), { p: [0, -.1, 0] });
  for (const m of [cork, stain]) { m.geometry.rotateZ(Math.PI / 2); m.geometry.rotateY(.7); m.geometry.translate(.86, .125, .3); }
  const tie = [torus(.142, .014, cord, { r: [Math.PI / 2, 0, 0], p: [0, 1.3, 0] }), rod([[.13, 1.3, .06], [.3, 1.22, .12], [.5, .7, .2], [.66, .26, .27], [.78, .2, .3]], .012, cord, { n: 30, sides: 6 }), ball(.024, cord, { p: [.13, 1.3, .06] })];
  const lab = sheet((u, v) => { const a = (u - .5) * 1.0 - .5, y = .22 + v * .3, R = .5 - Math.max(0, y - .16) * .09 + .006; return V(Math.sin(a) * R, y, Math.cos(a) * R); }, 20, 4, paper('#e4d8b8', { map: drawn(256, 256, (x, w, h) => { x.strokeStyle = 'rgba(40,26,60,.85)'; x.lineWidth = 6; x.lineCap = 'round'; x.beginPath(); x.moveTo(60, 170); x.bezierCurveTo(90, 20, 150, 210, 200, 60); x.stroke(); x.lineWidth = 3; for (let i = 0; i < 3; i++) { x.beginPath(); x.moveTo(50, 200 + i * 14); x.lineTo(206, 200 + i * 14); x.stroke(); } }) }));
  return item('Vial of bubbling ink', [part('vial', 'body', [0, 0, 0], [body]), part('ink', 'detail', [0, 0, .9], [fill, run, bead, blot].concat(bub)), part('what comes off it', 'detail', [0, .5, 0], vap), part('cork', 'trim', [.6, .2, 0], [cork, stain].concat(tie)), part('label', '', [-.4, 0, .6], [lab])], { stand: true, yaw: .2, el: 14, lights: [{ p: [0, .95, 0], color: c.detail, power: .9, flick: .12 }], halo: .7 });
}

export function conch(c, t) {
  const tt = t || 0, ph = tt * Math.PI * 2, TH = 6 * 2 * Math.PI, kg = Math.log(1.62) / (2 * Math.PI), N = 480, nA = 48, rings = [], lining = [], sm = 1 / Math.exp(kg * TH);
  const outer = shell(c.body, { metal: .12, rough: .3, irid: .4, bump: .7, double: true }), inner = matte(c.trim, { rough: .16, wet: 1, double: true });
  // A tube that grows as it winds: each turn is the last one again, larger and lower.
  let end = null;
  for (let i = 0; i <= N; i++) {
    const th = i / N * TH, s = Math.exp(kg * th) * sm, lip = ss((i / N - .94) / .06), a = .4 * s * (1 + .2 * lip), er = V(Math.cos(th), 0, Math.sin(th)), cc = er.clone().multiplyScalar(.39 * s).add(V(0, -1.9 * s, 0)), pts = [], pin = [];
    for (let j = 0; j < nA; j++) { const phi = j / nA * Math.PI * 2, dk = Math.atan2(Math.sin(phi - 1.15), Math.cos(phi - 1.15)), dc = Math.atan2(Math.sin(phi + Math.PI / 2), Math.cos(phi + Math.PI / 2)), k = 1 + Math.pow(Math.max(0, Math.cos(th * 4.5)), 6) * Math.exp(-dk * dk / .09) * .34 * ss(i / N * 3) + .022 * Math.cos(phi * 11) + .55 * Math.exp(-dc * dc / .12) * ss(i / N * 2); pts.push([Math.cos(phi) * a * k, Math.sin(phi) * a * 1.3 * k]); pin.push([Math.cos(phi) * a * k * .95, Math.sin(phi) * a * 1.3 * k * .95]); }
    rings.push(ring(cc, er, Y, pts)); if (i > N * .7) lining.push(ring(cc, er, Y, pin)); if (i === N) end = { c: cc, ring: rings[i] };
  }
  const body = M(loft(rings), outer), pink = M(loft(lining), inner), edge = M(tube(end.ring, .02, 8, true), outer);
  { const uv = body.geometry.attributes.uv; for (let i = 0; i < uv.count; i++) uv.setXY(i, uv.getX(i) * 3, uv.getY(i) * 30); }
  const song = [ball(.3, ghost(c.detail, .03 * (1 + .5 * Math.sin(ph))), { s: [1, 1.3, 1], p: [end.c.x, end.c.y, end.c.z - .3] }), ball(.04, glow(c.detail, .5 * (1 + .4 * Math.sin(ph)), { add: true }), { p: [end.c.x - .05, end.c.y + .05, end.c.z - .5] })];
  for (let k = 0; k < 3; k++) { const u = (tt + k / 3) % 1, m = torus(.24 + u * .34, .012, ghost(c.detail, .13 * (1 - u) * Math.min(1, u * 6)), { s: [1, 1.3, 1], p: [end.c.x, end.c.y, end.c.z + .12 + u * .7], seg: 48 }); m.userData.nofit = true; song.push(m); }
  return item('Singing shell', [part('shell', 'body', [0, 0, 0], [body, edge]), part('mother-of-pearl', 'trim', [.5, -.3, .3], [pink]), part('the song still in it', 'detail', [0, 0, .9], song)], { yaw: .5, fill: .9, lights: [{ p: [end.c.x, end.c.y, end.c.z - .1], color: c.detail, power: .22 }], halo: .6 });
}

// A root: a tapering tube with lumps and the ring wrinkles a root gets as it thickens.
function gnarl(pts, rf, mat, o) {
  o = o || {}; const nz = makeNoise(o.seed || 7, 32), P = path(pts, o.n || 40), sides = o.sides || 18, g = tube(P, rf, sides), p = g.attributes.position, nm = g.attributes.normal, R = P.length;
  for (let i = 0; i < p.count; i++) { const s = Math.floor(i / (sides + 1)) / (R - 1), a = (i % (sides + 1)) / sides * Math.PI * 2, d = (fbm(nz, Math.cos(a) * 1.4 + 4, s * (o.len || 6) + Math.sin(a) * 1.4, 3) - .5) * (o.lump == null ? .6 : o.lump) * rf(s) + Math.sin(s * (o.rings || 60) + Math.cos(a) * 2.5 + fbm(nz, s * 9, Math.cos(a) + 2, 2) * 4) * .014 * Math.min(1, rf(s) / .08); p.setXYZ(i, p.getX(i) + nm.getX(i) * d, p.getY(i) + nm.getY(i) * d, p.getZ(i) + nm.getZ(i) * d); }
  g.computeVertexNormals(); const q = g.attributes.normal; for (let i = 0; i < R; i++) { const a = i * (sides + 1), b = a + sides, x = q.getX(a) + q.getX(b), y = q.getY(a) + q.getY(b), z = q.getZ(a) + q.getZ(b), l = Math.hypot(x, y, z) || 1; q.setXYZ(a, x / l, y / l, z / l); q.setXYZ(b, x / l, y / l, z / l); }
  const m = M(g, mat); m.userData.spine = P; return m;
}

export function mandrake(c) {
  const skin = leather(c.body, { bump: 1.7 }), twine = matte(c.trim, { rough: .9, bump: .8 }), leafM = matte(c.detail, { rough: .5, sheen: .3, bump: 1.2, double: true }), r = rng(31), n = makeNoise(5, 16);
  const torso = gnarl([[0, .84, .26], [-.02, .5, .27], [.03, .1, .27], [.0, -.3, .24], [-.02, -.52, .2]], s => .14 + .12 * Math.pow(Math.sin(Math.PI * Math.min(1, s * .95 + .05)), .7) + .05 * Math.exp(-Math.pow((s - .12) / .1, 2)), skin, { seed: 3, n: 50, sides: 24 });
  const limbs = [
    gnarl([[-.08, -.42, .2], [-.2, -.8, .13], [-.14, -1.2, .08], [-.27, -1.55, .035], [-.2, -1.84, .015]], s => .125 * Math.pow(1 - s, .9) + .008, skin, { seed: 5 }),
    gnarl([[.09, -.42, .2], [.25, -.75, .13], [.31, -1.1, .08], [.2, -1.45, .04], [.31, -1.72, .015]], s => .115 * Math.pow(1 - s, .9) + .008, skin, { seed: 6 }),
    gnarl([[-.2, .42, .26], [-.46, .3, .16], [-.66, .0, .08], [-.7, -.32, .03], [-.87, -.52, .012]], s => .078 * Math.pow(1 - s, .8) + .006, skin, { seed: 8, rings: 40 }),
    gnarl([[.2, .38, .26], [.5, .42, .16], [.74, .24, .08], [.9, .32, .03], [1.06, .2, .012]], s => .07 * Math.pow(1 - s, .8) + .006, skin, { seed: 9, rings: 40 })];
  const hips = blob([.2, .16, .17], v => 1 + (n(v.x * 2 + 3, v.y * 2) - .5) * .3, skin, { p: [0, -.44, .2] }), crown = blob([.2, .13, .19], v => 1 + (n(v.x * 3, v.z * 3 + v.y) - .5) * .35, skin, { p: [0, .86, .26] });
  const hairs = []; for (const L of limbs) { const P = L.userData.spine; for (let k = 0; k < 7; k++) { const i = 8 + Math.floor(r() * (P.length - 10)), q = P[i], a = r() * 6.28, len = .12 + r() * .26, d = V(Math.cos(a), Math.sin(a), 0); hairs.push(rod([[q.x, q.y, q.z], [q.x + d.x * len * .5, q.y + d.y * len * .5 - .02, Math.max(.01, q.z * .5)], [q.x + d.x * len + (r() - .5) * .08, q.y + d.y * len + (r() - .5) * .08, .008]], s => .013 * (1 - s) + .002, skin, { n: 10, sides: 5 })); } }
  const leaves = [], ribs = [];
  for (let i = 0; i < 7; i++) { const a = (i - 3) * .36 + (r() - .5) * .14, L = .62 + r() * .4, w = .15 + r() * .06, dx = Math.sin(a), dy = Math.cos(a), bend = (r() - .5) * .5, at = (u, v) => { const k = v * L, side = (u - .5) * 2 * w * Math.pow(Math.sin(Math.PI * Math.pow(v, .8) * .97 + .03), .8), cx = dx * k + dy * bend * v * v * .4, cy = dy * k - dx * bend * v * v * .4; return V(cx + dy * side, .94 + cy - dx * side, .3 * Math.pow(1 - v, 1.6) + .012 + Math.abs(u - .5) * .2 * Math.sin(Math.PI * v) + Math.sin(v * 22 + i) * .03 * Math.abs(u - .5) * 2 + .07 * Math.sin(v * 3 + i * 2) * v); };
    leaves.push(sheet(at, 12, 40, leafM)); ribs.push(rod(Array.from({ length: 14 }, (_, j) => at(.5, j / 13).add(V(0, 0, .004))), s => .012 * (1 - s) + .003, leafM, { sides: 5 })); }
  const hel = []; for (let i = 0; i <= 120; i++) { const s = i / 120, a = s * Math.PI * 2 * 3.2 + 1, y = .22 - s * .2; hel.push(V(Math.cos(a) * .285, y, .27 + Math.sin(a) * .285)); }
  const bind = [M(tube(hel, .016, 6), twine), rod([[.2, .18, .5], [.36, .3, .5], [.5, .22, .3], [.62, .3, .04]], .016, twine, { n: 16, sides: 6 }), ball(.035, twine, { p: [.2, .18, .49] })];
  const soil = many(new THREE.DodecahedronGeometry(1, 0), matte('#2a1f17', { rough: .95, flat: true }), Array.from({ length: 40 }, () => ({ p: [(r() - .5) * 1.6, -.4 - r() * 1.4, .012], r: [r() * 3, r() * 3, 0], s: .008 + r() * .02 })));
  return item('Mandrake root', [part('root', 'body', [0, 0, 0], [torso, hips, crown].concat(limbs, hairs, [soil])), part('twine', 'trim', [.5, 0, .4], bind), part('leaves', 'detail', [0, .6, 0], leaves.concat(ribs))], { yaw: .22, fill: .92 });
}

export function egg(c, t) {
  const tt = t || 0, beat = Math.exp(-Math.pow((tt - .15) / .07, 2)) + .6 * Math.exp(-Math.pow((tt - .4) / .08, 2)), sc = shell(c.body, { metal: .4, rough: .3, bump: .7, irid: .7 }), H = 1.5, y0 = .16, r = rng(23);
  const pr = s => .56 * Math.pow(Math.sin(Math.PI * Math.min(.998, Math.max(.002, s))), .62) * (1 - .28 * s);
  const emb = matte('#1c0f0a', { rough: .6 }); emb.emissive = C(c.detail); emb.emissiveIntensity = .3 + 1.2 * beat;
  const core = turned(Array.from({ length: 41 }, (_, i) => [pr(i / 40) * .95, y0 + i / 40 * H]), emb, { sm: 0, seg: 64 });
  // One scale is a small shield, narrow at its tip, with a keel. Each row covers the roots of the row above.
  const sg = new THREE.SphereGeometry(1, 14, 10), sp = sg.attributes.position; for (let i = 0; i < sp.count; i++) { const x = sp.getX(i), y = sp.getY(i), z = sp.getZ(i), w = 1 - .5 * Math.max(0, y) * Math.max(0, y) - .15 * Math.max(0, -y); sp.setXYZ(i, x * w * .6, y * .62, z * .16 + (z > 0 ? .07 * Math.exp(-x * x / .05) * (1 + y) * .5 : 0)); } sg.computeVertexNormals();
  const list = [], m4 = new THREE.Matrix4();
  for (let i = 17; i >= 0; i--) {
    const s = .04 + i * .055, R = pr(s), cnt = Math.max(5, Math.round(2 * Math.PI * R / .21)), ds = 1e-3, T = V(pr(s + ds) - pr(s - ds), 2 * ds * H, 0).normalize(), size = .13 + .1 * R / .56;
    for (let j = 0; j < cnt; j++) { const a = (j + (i % 2) * .5) / cnt * Math.PI * 2 + i * .13, er = V(Math.cos(a), 0, Math.sin(a)), A = V(-Math.sin(a), 0, Math.cos(a)), Tw = er.clone().multiplyScalar(T.x).add(V(0, T.y, 0)).normalize(), Nw = new THREE.Vector3().crossVectors(Tw, A).normalize(), tl = .2 + r() * .08, Tt = Tw.clone().multiplyScalar(Math.cos(tl)).addScaledVector(Nw, Math.sin(tl)), Nt = Nw.clone().multiplyScalar(Math.cos(tl)).addScaledVector(Tw, -Math.sin(tl)), Xb = new THREE.Vector3().crossVectors(Tt, Nt);
      list.push({ p: er.clone().multiplyScalar(R + .014).add(V(0, y0 + s * H, 0)).addScaledVector(Tt, size * .3), q: new THREE.Quaternion().setFromRotationMatrix(m4.makeBasis(Xb, Tt, Nt)), s: size * (.92 + r() * .16) }); }
  }
  const scales = many(sg, sc, list);
  const strawM = matte(c.trim, { rough: .8 }), bed = blob([1.02, .07, 1.02], null, matte('#' + C(c.trim).multiplyScalar(.5).getHexString(), { rough: .95, bump: 1 }), { p: [0, .06, 0] });
  const straw = many(new THREE.CylinderGeometry(.5, .5, 1, 5), strawM, Array.from({ length: 340 }, () => { const a = r() * 6.28, d = .52 + r() * .52, h = r(), dir = V(-Math.sin(a) + (r() - .5) * .9, (r() - .5) * .4, Math.cos(a) + (r() - .5) * .9).normalize(); return { p: [Math.cos(a) * d, .06 + h * .3 * (1 - Math.abs(d - .76) / .45), Math.sin(a) * d], q: aim(dir), s: [.013 + r() * .011, .5 + r() * .7, .013 + r() * .011] }; }));
  const motes = Array.from({ length: 7 }, () => { const a = r() * 6.28, d = .5 + r() * .25, h = (r() + tt) % 1, m = ball(.009 + r() * .008, glow(c.detail, 2.2 * (1 - h) * Math.min(1, h * 6), { add: true }), { p: [Math.cos(a) * d, .4 + h * 1.2, Math.sin(a) * d], seg: 8, seg2: 6 }); m.userData.nofit = true; return m; });
  return item('Dragon egg', [part('scales', 'body', [0, .25, 0], [scales]), part('nest', 'trim', [0, -.35, 0], [bed, straw]), part('what is warm inside', 'detail', [0, 0, 1.0], [core].concat(motes))], { stand: true, yaw: .3, el: 16, lights: [{ p: [0, .8, 1.0], color: c.detail, power: .2 + .55 * beat }], halo: .7 });
}

export function wing(c, t) {
  const tt = t || 0, ph = tt * Math.PI * 2, r = rng(17);
  const mem = new THREE.MeshPhysicalMaterial({ color: C(c.body), transparent: true, opacity: .38, roughness: .1, metalness: 0, iridescence: 1, iridescenceIOR: 1.5, iridescenceThicknessRange: [180, 520], clearcoat: 1, clearcoatRoughness: .08, envMapIntensity: 2.2, side: THREE.DoubleSide, depthWrite: false }); mem.userData = { label: '', noshadow: true };
  const veinM = matte(c.trim, { rough: .4, wet: .4 }), root = [-.98, -.1], zf = (x, y) => .035 + .03 * Math.sin(x * 2.6 + y * 2) + .05 * Math.max(0, x) * Math.max(0, x) * .5;
  const blades = [], veins = [], spots = [];
  const mk = (out, cx, cy, nv, bend) => {
    blades.push(fan(out, zf, mem, { nu: 120, nv: 14, cx: cx, cy: cy })); veins.push(rim(out, zf, .011, veinM, { lift: .003, n: 160 }));
    const cv = new THREE.CatmullRomCurve3(out.map(p => V(p[0], p[1], 0)), true), at = (k, s) => { const T = cv.getPointAt(.14 + k / (nv - 1) * .72), x = root[0] + (T.x - root[0]) * s, y = root[1] + (T.y - root[1]) * s, b = Math.sin(Math.PI * s) * bend * (k / (nv - 1) - .5); return V(x - (T.y - root[1]) * b, y + (T.x - root[0]) * b, 0); }, up = q => { q.z = zf(q.x, q.y) + .004; return q; };
    for (let k = 0; k < nv; k++) { veins.push(rod(Array.from({ length: 24 }, (_, j) => up(at(k, j / 23))), s => .014 * (1 - s * .6), veinM, { sides: 6 })); if (k) for (let j = 1; j < 9; j++) { const s = .16 + j * .095 + (r() - .5) * .05; veins.push(rod([up(at(k - 1, s)), up(at(k, Math.min(1, s + (r() - .3) * .08)))], .007, veinM, { sides: 5 })); } }
    for (let i = 0; i < 60; i++) { const T = cv.getPointAt(r()), f = Math.sqrt(r()) * .92, x = cx + (T.x - cx) * f, y = cy + (T.y - cy) * f; spots.push([x, y, zf(x, y) + .012]); }
  };
  mk([[-.98, -.08], [-.6, .14], [-.1, .36], [.5, .52], [1.0, .56], [1.3, .44], [1.38, .24], [1.2, .05], [.7, -.08], [.1, -.15], [-.5, -.17]], .3, .2, 7, .2);
  mk([[-.98, -.12], [-.55, -.23], [0, -.27], [.5, -.3], [.9, -.42], [1.0, -.62], [.85, -.8], [.5, -.84], [.05, -.72], [-.4, -.5], [-.75, -.3]], .2, -.52, 6, -.15);
  const stub = blob([.1, .07, .05], null, matte('#3a2a30', { rough: .4, wet: .6 }), { p: [-1.0, -.1, .05] });
  for (let i = 0; i < 9; i++) { const a = r() * 6.28, d = Math.sqrt(r()); spots.push([1.2 + Math.cos(a) * d * .36, -.3 + Math.sin(a) * d * .28, .02]); }
  const dust = [0, 1, 2].map(k => many(new THREE.SphereGeometry(1, 6, 4), glow(c.detail, .5 + 3.2 * Math.pow(.5 + .5 * Math.sin(ph + k * 2.094), 3)), spots.filter((_, i) => i % 3 === k).map(q => ({ p: q, s: .006 + r() * .008 }))));
  // What has shaken loose lies as grains on the table, not as a heap.
  const grains = many(new THREE.SphereGeometry(1, 6, 4), matte('#' + C(c.detail).multiplyScalar(.5).getHexString(), { rough: .8 }), Array.from({ length: 70 }, () => { const a = r() * 6.28, d = Math.pow(r(), .7); return { p: [1.2 + Math.cos(a) * d * .4, -.3 + Math.sin(a) * d * .3, .006], s: [.006 + r() * .008, .006 + r() * .008, .004] }; }));
  return item('Pixie wing', [part('wing', 'body', [0, 0, .35], blades), part('veins', 'trim', [0, 0, 0], veins.concat([stub])), part('dust', 'detail', [.3, -.3, .8], dust.concat([grains]))], { yaw: .2, fill: .9, lights: [{ p: [1.2, -.3, .3], color: c.detail, power: .35 }], halo: .8 });
}
