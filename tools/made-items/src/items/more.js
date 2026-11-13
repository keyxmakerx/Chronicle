// A butcher's cuts, more remnants, and more things of magical origin.
import { THREE, V, M, X, Y, Z, part, item, turned, lathe, smooth, rod, ball, torus, cyl, rbox, blob, sheet, fan, rim, chain, many, aim, crystalGeo, lampGlow, loft, ring, tube, path, strands, drawn, makeNoise, fbm, rng } from '../kit.js';
import { tex } from '../util.js';
import { metal, wood, bone, leather, cloth, rock, paper, matte, gem, glass, liquid, glow, ghost, dark, shell } from '../mats.js';

const C = hex => new THREE.Color(hex), enc = q => Math.pow(Math.max(0, Math.min(1, q)), 1 / 2.2), ss = x => { x = Math.max(0, Math.min(1, x)); return x * x * (3 - 2 * x); };
const mix = (a, b, k) => [a[0] + (b[0] - a[0]) * k, a[1] + (b[1] - a[1]) * k, a[2] + (b[2] - a[2]) * k], rgb = hex => { const c = C(hex); return [c.r, c.g, c.b]; };
const seg = (x, z, a, b) => { const dx = b[0] - a[0], dz = b[1] - a[1], k = Math.max(0, Math.min(1, ((x - a[0]) * dx + (z - a[1]) * dz) / (dx * dx + dz * dz))); return Math.hypot(x - a[0] - dx * k, z - a[1] - dz * k); };
// Light laid over a solid as an added layer, bright where fn(vertex index) says.
function lit(mesh, hex, fn) {
  const g = mesh.geometry.clone(), n = g.attributes.position.count, col = new Float32Array(n * 3), c = C(hex);
  for (let i = 0; i < n; i++) { const f = fn(i, g); col[i * 3] = c.r * f; col[i * 3 + 1] = c.g * f; col[i * 3 + 2] = c.b * f; }
  g.setAttribute('color', new THREE.BufferAttribute(col, 3));
  const m = new THREE.Mesh(g, new THREE.MeshBasicMaterial({ vertexColors: true, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false })); m.userData.glow = true; m.userData.nofit = true; m.userData.noshadow = true; m.renderOrder = 1; return m;
}

// A slice of something lying flat: a level top inside an outline, a rounded shoulder and a wall down to the table.
// The picture of the cut face is laid on by position, so the wall takes the colour of the edge above it.
function slice(outline, h, face, o) {
  o = o || {}; const nu = 160, cv = new THREE.CatmullRomCurve3(outline.map(p => V(p[0], 0, p[1])), true, 'catmullrom', .5), rp = cv.getSpacedPoints(nu), b = Math.min(h * .4, .05), n = makeNoise(o.seed || 3, 32);
  let x0 = 9, x1 = -9, z0 = 9, z1 = -9; for (const q of rp) { x0 = Math.min(x0, q.x); x1 = Math.max(x1, q.x); z0 = Math.min(z0, q.z); z1 = Math.max(z1, q.z); }
  const W = (x1 - x0) * 1.04, H = (z1 - z0) * 1.04, cx = (x0 + x1) / 2, cz = (z0 + z1) / 2, edge = (x, z) => { let d = 9; for (let i = 0; i < nu; i += 2) d = Math.min(d, Math.hypot(x - rp[i].x, z - rp[i].z)); return d; };
  const mat = matte('#ffffff', { rough: o.rough == null ? .4 : o.rough, wet: o.wet == null ? .55 : o.wet, bump: .5, double: true });
  mat.map = tex(384, 384, (u, v) => { const x = cx + (u - .5) * W, z = cz + (v - .5) * H; return face(x, z, edge(x, z)).map(enc); }, { srgb: true });
  const m = sheet((u, v) => {
    const q = rp[Math.round(u * nu) % nu]; let f, y;
    if (v < .7) { f = v / .7 * .93; y = h; } else if (v < .86) { const a = (v - .7) / .16 * Math.PI / 2; f = .93 + .07 * Math.sin(a); y = h - b + b * Math.cos(a); } else { f = 1; y = (h - b) * (1 - (v - .86) / .14); }
    const x = cx + (q.x - cx) * f, z = cz + (q.z - cz) * f; return V(x, y + (v < .86 ? (fbm(n, x * 4 + 2, z * 4, 3) - .5) * (o.lump == null ? .03 : o.lump) : 0), z);
  }, nu, 26, mat);
  const p = m.geometry.attributes.position, uv = m.geometry.attributes.uv; // The wall and shoulder take their colour from a little way in, so a thin skin at the very edge does not stripe them.
  for (let i = 0; i < p.count; i++) { const v = Math.floor(i / (nu + 1)) / 26, f = v < .7 ? 1 : .88 / (v < .86 ? .93 + .07 * Math.sin((v - .7) / .16 * Math.PI / 2) : 1); uv.setXY(i, (p.getX(i) - cx) * f / W + .5, (p.getZ(i) - cz) * f / H + .5); }
  return m;
}

export function meat(c) {
  const n = makeNoise(21, 64), n2 = makeNoise(47, 64), r = rng(7), lean = rgb(c.body).map(q => q * .62), dkl = [lean[0] * .55, lean[1] * .4, lean[2] * .45], fat = [.72, .62, .44], bnc = [.66, .6, .45], herbM = matte(c.detail, { rough: .75 }), pap = paper(c.trim), bn = bone('#d8ccac', { gloss: .3 });
  const flesh = (x, z, k) => { const g = fbm(n, x * 4 + 3, z * 4, 4), fib = fbm(n2, x * 12, z * 40 + 5, 3), ridge = 1 - Math.abs(2 * fbm(n2, x * 9 + 9 + g * 3, z * 20 + g * 4, 4) - 1); return mix(mix(lean, dkl, ss((g - .3) * 1.8) * .8 + (fib - .5) * .4), fat, ss((ridge - .93) / .06) * .55 * k * ss((g - .25) * 3)); };
  const suet = (x, z) => mix(fat, [.74, .46, .36], Math.max(0, fbm(n, x * 8 + 7, z * 8, 3) - .48) * 1.2);
  // A steak cut across the spine: the bone is a T, the large muscle on one side of it and the small on the other.
  const T = [[[-.4, -.53], [.42, -.52]], [[.04, -.5], [.1, -.08]]];
  const steak = slice([[-.7, -.16], [-.5, -.46], [-.14, -.55], [.3, -.53], [.6, -.4], [.74, -.02], [.6, .3], [.24, .44], [-.24, .5], [-.62, .3]], .13, (x, z, d) => {
    const wob = (n(x * 14, z * 14) - .5) * .02, db = Math.min(seg(x, z, T[0][0], T[0][1]) - .085, seg(x, z, T[1][0], T[1][1]) - (.06 - .03 * ss((z + .45) / .4))) + wob;
    if (db < 0) return mix(mix(bnc, [.5, .42, .3], n(x * 30, z * 30) * .5), [.34, .13, .11], ss((.03 - Math.hypot(x - .04, z + .44)) / .012) * .8);
    if (db < .016) return mix(lean, [.16, .04, .04], .6);
    const cap = ss((-x * .5 + z * .9 - .05) / .25), fw = .012 + cap * (.05 + .05 * ss((fbm(n, x * 2 + 1, z * 2, 2) - .35) * 3));
    if (d < fw) return suet(x, z);
    return flesh(x, z, x > .12 ? .3 : 1);
  }, { seed: 5, wet: .7 });
  steak.geometry.rotateY(.25); steak.geometry.translate(-.42, .03, -.15);
  const chop = slice([[-.34, -.1], [-.2, -.3], [.1, -.34], [.36, -.2], [.44, .05], [.3, .26], [.02, .32], [-.24, .2]], .15, (x, z, d) => d < .014 + ss((x * .8 + z * .5 + .05) / .3) * (.05 + .04 * ss((fbm(n, x * 3 + 5, z * 3, 2) - .3) * 3)) ? suet(x, z) : flesh(x + 3, z + 1, .7), { seed: 9, wet: .7 });
  const rib = rod([[.26, .085, -.33], [-.05, .085, -.37], [-.35, .085, -.31], [-.6, .085, -.2], [-.8, .085, -.08]], s => .062 * (1 + .12 * Math.exp(-Math.pow((1 - s) / .1, 2))), bn, { n: 24, sides: 14, s: [1, .78, 1] }), knob = cyl(.058, .058, .012, matte('#8a5a48', { rough: .5 }), { s: [1, 1, .78] }); knob.geometry.rotateZ(Math.PI / 2); knob.geometry.rotateY(.54); knob.geometry.translate(-.802, .066, -.078);
  for (const m of [chop, rib, knob]) { m.geometry.rotateY(3.3); m.geometry.translate(.62, .03, .36); }
  const sheetM = sheet((u, v) => { const x = (u - .5) * 2.7, z = (v - .5) * 1.95, e = Math.max(Math.abs(u - .5), Math.abs(v - .5)) * 2; return V(x + .12, .012 + (fbm(n2, u * 5, v * 4, 3) - .5) * .05 * (.3 + e) + Math.pow(e, 6) * .09 * (.5 + n(u * 6, v * 6)), z + .08); }, 60, 44, pap);
  const stem = rod([[-.72, .17, .0], [-.42, .185, .08], [-.14, .17, .2]], .012, wood('#5a4630', { bump: .2 }), { n: 16, sides: 6 });
  const needles = many(new THREE.SphereGeometry(1, 6, 4), herbM, Array.from({ length: 64 }, (_, i) => { const s = i / 63, x = -.72 + s * .58, z = .0 + s * .2, a = i * 2.4, d = V(Math.cos(a) * .7 + .5, .35 + r() * .3, Math.sin(a) * .9).normalize(); return { p: [x + d.x * .055, .18 + d.y * .05, z + d.z * .055], q: aim(d), s: [.012, .06 + r() * .02, .008] }; }));
  const salt = many(new THREE.BoxGeometry(1, 1, 1), matte('#f4f1ea', { rough: .3 }), Array.from({ length: 40 }, () => { const a = r() * 6.28, d = Math.sqrt(r()); return { p: [-.45 + Math.cos(a) * d * .42, .165 + r() * .008, -.12 + Math.sin(a) * d * .3], r: [r() * 3, r() * 3, r() * 3], s: .007 + r() * .009 }; }));
  const pepper = many(new THREE.SphereGeometry(.018, 8, 6), matte('#2a211c', { rough: .7 }), Array.from({ length: 9 }, () => ({ p: [.5 + r() * .7, .04, -.7 + r() * .35] })));
  return item('Cuts of meat', [part('paper', 'trim', [0, -.35, 0], [sheetM]), part('steak', 'body', [-.3, .1, 0], [steak, salt]), part('chop', 'body', [.5, .2, .3], [chop, rib, knob]), part('herb', 'detail', [0, .6, 0], [stem, needles, pepper])], { stand: true, yaw: .3, el: 42, fill: .92 });
}

export function hand(c, t) {
  const bn = bone(c.body, { rough: .75, gloss: .08 }), gold = metal(c.trim, { kind: 'cast', rough: .25 }), ph = (t || 0) * Math.PI * 2, tw = k => Math.pow(Math.max(0, Math.sin(ph + k)), 2), n = makeNoise(13, 16);
  const bones = [], joints = [], pull = [], gild = [];
  // A long bone is waisted in the middle and flared where it meets the next one.
  const long = (a, b, r) => { bones.push(rod([a, a.clone().lerp(b, .5), b], s => r * (.7 + .55 * Math.pow(Math.abs(2 * s - 1), 2.2)) * (1 - .1 * s), bn, { n: 12, sides: 10 })); joints.push({ p: a.clone(), s: [r * 1.3, r * 1.3, r * 1.05] }, { p: b.clone(), s: [r * 1.15, r * 1.15, r * .95] }); };
  const wrist = [[-.27, -.78], [-.09, -.74], [.08, -.75], [.24, -.8]];
  [[-.3, -.02, -.16, [.42, .26, .19], .05, .2, tw(0) * .55], [-.1, .06, -.04, [.47, .3, .2], .052, .18, tw(1.1) * .4], [.1, .03, .08, [.43, .28, .19], .05, .26, 0], [.28, -.08, .22, [.33, .2, .16], .044, .34, 0]].forEach((f, i) => {
    let p = V(f[0], f[1], .07), phi = .08; const line = [V(wrist[i][0], wrist[i][1], .1)]; long(V(wrist[i][0], wrist[i][1], .06), p, f[4] * 1.1); line.push(p.clone().add(V(0, 0, .06)));
    f[3].forEach((len, j) => { phi += f[5] * (j ? 1.25 : 1) + f[6] * (j ? 1 : .6); const d = V(Math.sin(f[2]) * Math.cos(phi), Math.cos(f[2]) * Math.cos(phi), Math.sin(phi)), q = p.clone().addScaledVector(d, len); long(p, q, f[4] * (1 - j * .15));
      if (i === 2 && !j) { const m = torus(.07, .017, gold), k = new THREE.Quaternion().setFromUnitVectors(Z, d), at = p.clone().lerp(q, .45); m.geometry.applyQuaternion(k); m.geometry.translate(at.x, at.y, at.z); gild.push(m, ball(.028, gem('#7a1f2a', { round: true, edges: 0, lit: .2 }), { p: [at.x, at.y - .01, at.z + .08] })); }
      p = q; line.push(p.clone().add(V(0, 0, .055))); });
    if (f[6] || i < 2) pull.push(rod(line, .008, ghost(c.detail, .22 + .6 * (i ? tw(1.1) : tw(0))), { n: 30, sides: 5 }));
  });
  { let p = V(-.6, -.42, .16); long(V(-.34, -.8, .07), p, .058); let phi = .25; [.3, .22].forEach((len, j) => { phi += .12; const d = V(-.62 * Math.cos(phi), .78 * Math.cos(phi), Math.sin(phi)).normalize(), q = p.clone().addScaledVector(d, len); long(p, q, .05 * (1 - j * .14)); p = q; }); }
  const carp = [[-.27, -.86, .09], [-.1, -.84, .085], [.07, -.85, .08], [.22, -.88, .075], [-.2, -.98, .085], [-.04, -.97, .09], [.12, -.98, .08], [.26, -.99, .07]].map((q, i) => blob([q[2] * 1.05, q[2] * .85, .065], v => 1 + (n(v.x * 2 + i * 3, v.y * 2 + v.z) - .5) * .5, bn, { p: [q[0], q[1], .07], seg: 20, seg2: 14 }));
  long(V(-.16, -1.1, .08), V(-.2, -1.86, .08), .078); long(V(.14, -1.1, .075), V(.17, -1.86, .075), .066);
  const knuckles = many(new THREE.SphereGeometry(1, 12, 8), bn, joints);
  const links = chain([[-.56, -1.16, .02], [-.3, -1.22, .13], [.0, -1.25, .17], [.3, -1.22, .12], [.5, -1.12, .02], [.62, -.92, .02]], .026, gold, { seed: 4 }), charm = cyl(.06, .06, .012, gold, { r: [Math.PI / 2, 0, 0], p: [.66, -.86, .012] });
  const haze = [ball(.5, ghost(c.detail, .035 * (1 + .5 * Math.sin(ph))), { s: [1, 1.2, .3], p: [0, -.4, .1] }), ball(.3, ghost(c.detail, .05 * (1 + .5 * Math.sin(ph))), { s: [1, 1, .3], p: [0, -.85, .1] })]; haze.forEach(m => { m.userData.nofit = true; });
  return item('Skeletal hand', [part('hand', 'body', [0, 0, 0], bones.concat(carp, [knuckles])), part('what it kept', 'trim', [.3, -.3, .5], gild.concat([links, charm])), part('what still pulls the fingers', 'detail', [0, .2, .7], pull)], { yaw: .32, fill: .92, lights: [{ p: [0, -.6, .5], color: c.detail, power: .5 }], halo: .8 });
}

export function geode(c, t) {
  const n = makeNoise(61, 32), ph = (t || 0) * Math.PI * 2, pw = 1 + .55 * Math.sin(ph), S = [.8, .68, .62], z0 = .4, r = rng(6), al = (x, y) => Math.pow(x / .16, 2) + Math.pow(y / .46, 2);
  const base = v => 1 + (fbm(n, v.x * 2.2 + 3, v.y * 2.2 + v.z * 2, 4) - .5) * .34 + (fbm(n, v.x * 8 + 1, v.y * 8 + v.z * 6, 3) - .5) * .07;
  // A stone nodule sawn across: the cut is a plane, and the hollow at its middle is where it once saw from.
  const f = v => { const k = base(v); if (v.z * k <= z0) return k; const a = al(v.x * z0 / v.z, v.y * z0 / v.z); return (z0 - (a < 1 ? .34 * Math.sqrt(1 - a) : 0)) / v.z; };
  const rind = blob(S, f, rock(c.body, { bump: 2.6, rough: .92 }), { seg: 200, seg2: 130 });
  const rho = a => { let q = .8; for (let i = 0; i < 5; i++) { const d = V(q * Math.cos(a), q * Math.sin(a), z0).normalize(), R = base(d); q = Math.sqrt(Math.max(.01, R * R - z0 * z0)); } return q; }, rc = Array.from({ length: 257 }, (_, i) => rho(i / 256 * Math.PI * 2));
  const out = Array.from({ length: 72 }, (_, i) => { const a = i / 72 * Math.PI * 2, q = rho(a) * .985; return [Math.cos(a) * q * S[0], Math.sin(a) * q * S[1]]; });
  const deep = rgb(c.trim), pale = mix(deep, [1, 1, 1], .72), dk = mix(deep, [0, 0, 0], .6), rindc = mix(rgb(c.body), [0, 0, 0], .3);
  const at = (u, v) => { const x = (u - .5) * 2.2 / S[0], y = (v - .5) * 2.2 / S[1], a = Math.atan2(y, x), aa = (a < 0 ? a + Math.PI * 2 : a) / (Math.PI * 2) * 256, i = Math.floor(aa), q = rc[i] + (rc[i + 1] - rc[i]) * (aa - i); return [x, y, Math.hypot(x, y) / q, a]; };
  const face = fan(out, () => z0 * S[2] + .004, matte('#ffffff', { rough: .16, wet: .9, bump: .1 }), { nu: 144, nv: 30 });
  face.material.map = tex(512, 512, (u, v) => {
    const [x, y, fr, a] = at(u, v), w = fr + (fbm(n, Math.cos(a) * 1.6 + 5, Math.sin(a) * 1.6 + fr * 2, 3) - .5) * .2, band = .5 + .5 * Math.sin(w * 30 + fbm(n, x * 3, y * 3, 2) * 8), hole = al(x, y);
    let col = mix(mix(dk, deep, band), pale, Math.pow(band, 10) * .55); col = mix(col, dk, ss((.55 - fr) / .5) * .5);
    if (fr > .88) col = mix(col, rindc, ss((fr - .88) / .08));
    if (hole < 2.2) col = mix(col, [.92, .95, .96], ss((2.2 - hole) / 1.0) * (.6 + .4 * n(x * 40, y * 40)));
    return col.map(enc);
  }, { srgb: true });
  face.material.alphaMap = tex(512, 512, (u, v) => { const [x, y] = at(u, v); return al(x, y) < 1 + (n(x * 30 + 2, y * 30) - .5) * .25 ? 0 : 1; }); face.material.alphaTest = .5; face.material.side = THREE.DoubleSide;
  { const p = face.geometry.attributes.position, uv = face.geometry.attributes.uv; for (let i = 0; i < p.count; i++) uv.setXY(i, p.getX(i) / 2.2 + .5, p.getY(i) / 2.2 + .5); }
  const gm = gem(c.detail, { vc: true, lit: .5 * pw, inner: .6 }), druse = [];
  for (let i = 0; i < 26; i++) { const y = (r() - .5) * .78, xm = .15 * Math.sqrt(Math.max(0, 1 - Math.pow(y / .44, 2))), x = (r() * 2 - 1) * xm, a = al(x, y), zz = z0 - .34 * Math.sqrt(Math.max(0, 1 - a)), len = .07 + r() * .12, m = M(crystalGeo(.022 + r() * .02, len, len * .4, { seed: i + 2, hex: c.detail }), gm, { r: [Math.PI / 2 + (r() - .5) * 1.1, 0, (r() - .5) * 1.1] }); m.geometry.translate(x * S[0], y * S[1], zz * S[2] - .01); druse.push(m); }
  const pupil = [ball(1, glow(c.detail, 1.1 * pw), { s: [.022, .2, .03], p: [0, 0, (z0 - .3) * S[2]] }), ball(1, ghost(c.detail, .1 * pw), { s: [.1, .3, .12], p: [0, 0, (z0 - .16) * S[2]] })];
  const all = [rind, face].concat(druse, pupil); all.forEach(m => { m.geometry.rotateX(-.32); m.geometry.rotateY(.12); m.geometry.translate(0, .62, 0); });
  const chips = Array.from({ length: 7 }, (_, i) => { const a = .4 + r() * 2.4, d = .85 + r() * .35; return M(new THREE.DodecahedronGeometry(.04 + r() * .05, 0), rock(c.body, { flat: true }), { r: [r() * 3, r() * 3, 0], p: [Math.cos(a) * d, .04, Math.sin(a) * d * .7 + .2] }); });
  return item('Stone eye', [part('rind', 'body', [0, 0, -.3], [rind].concat(chips)), part('rings', 'trim', [0, 0, .5], [face]), part('what it saw by', 'detail', [0, .1, 1.1], druse.concat(pupil))], { stand: true, yaw: .3, el: 14, lights: [{ p: [0, .7, .5], color: c.detail, power: .7 * pw }], halo: .8 });
}

