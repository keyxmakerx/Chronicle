// Building blocks for made items: turned, lofted, cut and draped solids, and
// the bookkeeping that lets every item be framed, lit and taken apart alike.
import * as THREE from 'three';
import { RoundedBoxGeometry } from 'three/examples/jsm/geometries/RoundedBoxGeometry.js';
import { loft, ring, tube, makeNoise, fbm, rng } from './util.js';

export { loft, ring, tube, makeNoise, fbm, rng, THREE };
export const X = new THREE.Vector3(1, 0, 0), Y = new THREE.Vector3(0, 1, 0), Z = new THREE.Vector3(0, 0, 1);
export function V(x, y, z) { return new THREE.Vector3(x, y, z || 0); }

// A mesh, optionally moved, turned and scaled in one go. The transform is baked
// into the geometry so parts can be taken apart by position alone.
export function M(geo, mat, o) {
  o = o || {};
  if (o.s) { const s = typeof o.s === 'number' ? [o.s, o.s, o.s] : o.s; geo.scale(s[0], s[1], s[2]); }
  if (o.r) { if (o.r[0]) geo.rotateX(o.r[0]); if (o.r[1]) geo.rotateY(o.r[1]); if (o.r[2]) geo.rotateZ(o.r[2]); }
  if (o.p) geo.translate(o.p[0], o.p[1], o.p[2] || 0);
  const m = new THREE.Mesh(geo, mat);
  if (o.order) m.renderOrder = o.order;
  if (o.noshadow) m.userData.noshadow = true;
  if (o.nofit) m.userData.nofit = true;
  return m;
}

// A named part: what it is called, which recipe colour made it, where it goes when taken apart.
export function part(name, role, ex, meshes) {
  const g = new THREE.Group(); g.name = name; g.userData = { role: role, ex: ex instanceof THREE.Vector3 ? ex : V(ex[0], ex[1], ex[2]) };
  const add = m => {
    if (Array.isArray(m)) { m.forEach(add); return; }
    if (!m) return;
    const mat = m.material, ud = mat && mat.userData || {};
    m.castShadow = !(m.userData.noshadow || ud.noshadow); m.receiveShadow = !ud.noshadow;
    if (ud.glow) m.userData.glow = true;
    g.add(m);
    // Clear solids are drawn as layers: the far wall first, then the tint, then the shine.
    if (ud.edges && !m.userData.layered && !m.isInstancedMesh) { const e = new THREE.LineSegments(new THREE.EdgesGeometry(m.geometry, 18), new THREE.LineBasicMaterial({ color: 0xffffff, transparent: true, opacity: ud.edges, blending: THREE.AdditiveBlending, depthWrite: false })); e.renderOrder = (m.renderOrder || 0) + 3; g.add(e); }
    if (ud.layers && !m.userData.layered) for (const l of ud.layers) { const c = new THREE.Mesh(m.geometry, l.mat); c.renderOrder = (m.renderOrder || 0) + l.order; c.userData.layered = true; c.userData.nofit = true; c.castShadow = false; c.receiveShadow = false; g.add(c); }
  };
  add(meshes);
  return g;
}

export function smooth(pts, n) { const c = new THREE.SplineCurve(pts.map(p => new THREE.Vector2(p[0], p[1]))); return c.getPoints(n || 48).map(p => [Math.max(0, p.x), p.y]); }
// A profile has to run upward on its outer side for the surface to face out, so it is turned round if it does not.
export function lathe(profile, seg) { let s = 0; for (let i = 0; i < profile.length - 1; i++) s += (profile[i + 1][1] - profile[i][1]) * (profile[i][0] + profile[i + 1][0]); const pr = s < 0 ? profile.slice().reverse() : profile; return new THREE.LatheGeometry(pr.map(p => new THREE.Vector2(p[0], p[1])), seg || 56); }
// A turned solid from a profile of [radius, height] pairs. sm rounds the profile.
export function turned(profile, mat, o) { o = o || {}; return M(lathe(o.sm === 0 ? profile : smooth(profile, o.sm || 64), o.seg), mat, o); }
export function path(pts, n, closed) { return new THREE.CatmullRomCurve3(pts.map(p => p instanceof THREE.Vector3 ? p : V(p[0], p[1], p[2])), !!closed).getPoints(n || 40); }
// A rod or cord along points. radius may be a function of 0..1 along it.
export function rod(pts, radius, mat, o) { o = o || {}; const p = pts[0] instanceof THREE.Vector3 ? pts : pts.map(q => V(q[0], q[1], q[2])); return M(tube(o.n ? path(p, o.n, o.closed) : p, radius, o.sides || 12, o.closed), mat, o); }
export function ball(r, mat, o) { return M(new THREE.SphereGeometry(r, (o && o.seg) || 32, (o && o.seg2) || 22), mat, o); }
export function torus(R, r, mat, o) { return M(new THREE.TorusGeometry(R, r, 14, (o && o.seg) || 48, (o && o.arc) || Math.PI * 2), mat, o); }
export function cyl(r0, r1, h, mat, o) { return M(new THREE.CylinderGeometry(r1, r0, h, (o && o.seg) || 40, 1, !!(o && o.open)), mat, o); }
export function rbox(w, h, d, r, mat, o) { return M(new RoundedBoxGeometry(w, h, d, 4, Math.min(r, w / 2, h / 2, d / 2)), mat, o); }

// A flat outline given thickness and a rounded edge: plates, keys, axe heads.
// pts is a list of [x, y]; curved makes it a smooth closed curve through them.
export function shape(pts, curved) {
  const s = new THREE.Shape();
  if (curved) { const c = new THREE.SplineCurve(pts.concat([pts[0]]).map(p => new THREE.Vector2(p[0], p[1]))); const q = c.getPoints(pts.length * 8); s.moveTo(q[0].x, q[0].y); for (let i = 1; i < q.length - 1; i++) s.lineTo(q[i].x, q[i].y); }
  else { s.moveTo(pts[0][0], pts[0][1]); for (let i = 1; i < pts.length; i++) s.lineTo(pts[i][0], pts[i][1]); }
  s.closePath(); return s;
}
export function hole(sh, pts, curved) { const h = shape(pts, curved); sh.holes.push(new THREE.Path(h.getPoints())); return sh; }
export function circle(x, y, r, n) { const p = []; for (let i = 0; i < (n || 32); i++) { const a = i / (n || 32) * Math.PI * 2; p.push([x + Math.cos(a) * r, y + Math.sin(a) * r]); } return p; }
export function slab(sh, depth, bevel, mat, o) {
  o = o || {}; if (Array.isArray(sh)) sh = shape(sh, o.curved);
  const b = Math.min(bevel, depth / 2 - 1e-4), g = new THREE.ExtrudeGeometry(sh, { depth: depth - 2 * b, bevelEnabled: bevel > 0, bevelThickness: b, bevelSize: o.bevelSize == null ? b : o.bevelSize, bevelSegments: o.bseg || 4, curveSegments: 24, steps: 1 });
  g.translate(0, 0, -(depth - 2 * b) / 2);
  const uv = g.attributes.uv; for (let i = 0; i < uv.count; i++) uv.setXY(i, uv.getX(i) * (o.uv || 1), uv.getY(i) * (o.uv || 1));
  return M(g, mat, o);
}

// A lump: a sphere pushed in and out by fn(direction) and stretched to size.
export function blob(size, fn, mat, o) {
  o = o || {}; const g = new THREE.SphereGeometry(1, o.seg || 64, o.seg2 || 44), p = g.attributes.position, v = new THREE.Vector3();
  for (let i = 0; i < p.count; i++) { v.fromBufferAttribute(p, i); const k = fn ? fn(v) : 1; p.setXYZ(i, v.x * k * size[0], v.y * k * size[1], v.z * k * size[2]); }
  g.computeVertexNormals(); weld(g, o.seg || 64, o.seg2 || 44); return M(g, mat, o);
}
function weld(g, ws, hs) { const n = g.attributes.normal; for (let y = 0; y <= hs; y++) { const a = y * (ws + 1), b = a + ws, x = n.getX(a) + n.getX(b), yy = n.getY(a) + n.getY(b), z = n.getZ(a) + n.getZ(b), l = Math.hypot(x, yy, z) || 1; n.setXYZ(a, x / l, yy / l, z / l); n.setXYZ(b, x / l, yy / l, z / l); } }

// A thin sheet from fn(u, v) -> point: cloth, paper, vanes, hides.
export function sheet(fn, nu, nv, mat, o) {
  const pos = [], uv = [], idx = [];
  for (let j = 0; j <= nv; j++) for (let i = 0; i <= nu; i++) { const p = fn(i / nu, j / nv); pos.push(p.x, p.y, p.z); uv.push(i / nu * ((o && o.uvx) || 1), j / nv * ((o && o.uvy) || 1)); }
  for (let j = 0; j < nv; j++) for (let i = 0; i < nu; i++) { const a = j * (nu + 1) + i, b = a + 1, c = a + nu + 1, d = c + 1; idx.push(a, b, c, b, d, c); }
  const g = new THREE.BufferGeometry(); g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3)); g.setAttribute('uv', new THREE.Float32BufferAttribute(uv, 2)); g.setIndex(idx); g.computeVertexNormals();
  return M(g, mat, o);
}

// Chain: oval links, each turned a quarter from the last, along a path.
export function chain(pts, size, mat, o) {
  o = o || {}; const c = new THREE.CatmullRomCurve3(pts.map(p => p instanceof THREE.Vector3 ? p : V(p[0], p[1], p[2]))), len = c.getLength(), step = size * 2.15, count = Math.max(2, Math.floor(len / step));
  const lg = new THREE.TorusGeometry(size, size * .3, 8, 20); lg.scale(1.5, 1, 1);
  const links = new THREE.InstancedMesh(lg, mat, count), q = new THREE.Quaternion(), q2 = new THREE.Quaternion(), one = V(1, 1, 1), m4 = new THREE.Matrix4(), r = rng(o.seed || 7);
  for (let i = 0; i < count; i++) { const u = (i + .5) / count, p = c.getPointAt(u), t = c.getTangentAt(u); q.setFromUnitVectors(X, t); q2.setFromAxisAngle(X, (i % 2 ? Math.PI / 2 : 0) + (r() - .5) * .5); q.multiply(q2); m4.compose(p, q, one); links.setMatrixAt(i, m4); }
  if (o.nofit) links.userData.nofit = true;
  return links;
}
// Many copies of one small solid: studs, beads, seeds, spikes. at is a list of {p, q|r, s}.
export function many(geo, mat, at) {
  const im = new THREE.InstancedMesh(geo, mat, at.length), m4 = new THREE.Matrix4(), q = new THREE.Quaternion(), e = new THREE.Euler(), s = new THREE.Vector3();
  at.forEach((a, i) => { if (a.q) q.copy(a.q); else if (a.r) q.setFromEuler(e.set(a.r[0], a.r[1], a.r[2])); else q.identity(); const k = a.s == null ? 1 : a.s; if (typeof k === 'number') s.set(k, k, k); else s.set(k[0], k[1], k[2]); m4.compose(a.p instanceof THREE.Vector3 ? a.p : V(a.p[0], a.p[1], a.p[2]), q, s); im.setMatrixAt(i, m4); });
  return im;
}
export function aim(dir) { return new THREE.Quaternion().setFromUnitVectors(Y, dir.clone().normalize()); }

// A double-edged blade as a loft: width and thickness given along its length.
export function blade(L, wf, tf, o) {
  o = o || {}; const N = o.n || 56, rings = [];
  for (let i = 0; i <= N; i++) {
    let s = i / N; s = 1 - Math.pow(1 - s, 1.5);
    const w = Math.max(1e-4, wf(s)), t = Math.max(5e-5, tf(s)), f = o.fuller ? Math.min(w * .6, o.fuller(s)) : 0, d = f > .002 ? t * .5 : 0, fw = Math.max(f, w * .12);
    rings.push(ring(V(0, s * L, 0), X, Z, [[w, 0], [fw, t], [0, t - d], [-fw, t], [-w, 0], [-fw, -t], [0, -(t - d)], [fw, -t]]));
  }
  const g = loft(rings, { sharp: true }); const uv = g.attributes.uv.array; for (let i = 0; i < uv.length; i += 2) { uv[i] *= 2; uv[i + 1] *= (o.uvy || 4); }
  return g;
}

// A grown crystal: a six-sided column that narrows a little and ends in a point.
export function crystalGeo(r, len, tip, o) {
  o = o || {}; const n = o.sides || 6, rr = rng(o.seed || 1), pos = [], top = [], bot = [], sh = [];
  for (let i = 0; i < n; i++) { const a = (i + (rr() - .5) * .5) / n * Math.PI * 2, k = .8 + rr() * .4; bot.push(V(Math.cos(a) * r * k, 0, Math.sin(a) * r * k)); top.push(V(Math.cos(a) * r * k * (o.taper || .86), len - tip * (.75 + rr() * .5), Math.sin(a) * r * k * (o.taper || .86))); sh.push(rr()); }
  const apex = V((rr() - .5) * r * .5, len, (rr() - .5) * r * .5), base = V(0, -r * .3, 0), tri = (a, b, c) => pos.push(a.x, a.y, a.z, b.x, b.y, b.z, c.x, c.y, c.z);
  for (let i = 0; i < n; i++) { const j = (i + 1) % n; tri(bot[i], top[i], bot[j]); tri(bot[j], top[i], top[j]); tri(top[i], apex, top[j]); tri(bot[j], base, bot[i]); }
  const g = new THREE.BufferGeometry(); g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3)); g.computeVertexNormals();
  if (o.hex) { const c = new THREE.Color(o.hex), pale = c.clone().lerp(new THREE.Color('#ffffff'), .7), deep = c.clone().multiplyScalar(.75), col = [], t = new THREE.Color(); for (let i = 0; i < pos.length; i += 3) { const s = Math.max(0, Math.min(1, pos[i + 1] / len)); t.copy(pale).lerp(c, Math.min(1, s * 2.4)).lerp(deep, Math.max(0, s - .5) * 1.2); col.push(t.r, t.g, t.b); } g.setAttribute('color', new THREE.Float32BufferAttribute(col, 3)); }
  return g;
}

// Wrap finished parts into an item. The framing box is measured from the parts
// unless one is given; trailing chains and haze can be left out of it with nofit.
export function item(name, parts, o) {
  o = o || {}; const g = new THREE.Group(); parts.forEach(p => p && g.add(p)); g.updateMatrixWorld(true);
  let fit = o.fit;
  if (!fit) { fit = new THREE.Box3(); g.traverse(m => { if (!m.isMesh || m.userData.nofit) return; if (m.isInstancedMesh) m.computeBoundingBox(); else m.geometry.computeBoundingBox(); fit.union((m.isInstancedMesh ? m.boundingBox : m.geometry.boundingBox).clone().applyMatrix4(m.matrixWorld)); }); }
  const c = fit.getCenter(new THREE.Vector3()), sz = fit.getSize(new THREE.Vector3());
  return Object.assign({ group: g, name: name, fit: fit, centre: c, radius: sz.length() / 2, stand: false, yaw: 0, fill: .84, lights: [] }, o, { fit: fit });
}

// A flame: nested teardrops that add their light, white at the heart, coloured
// at the edge, gone at the tip. t (0..1) is where it is in its flicker.
export function flame(h, r, hex, t, mk) {
  const out = [], c = new THREE.Color(hex), ph = (t || 0) * Math.PI * 2;
  for (const [k, pw, wh] of [[1, 1.5, 0], [.68, 2.2, .45], [.4, 4, .9]]) {
    const prof = []; for (let i = 0; i <= 28; i++) { const s = i / 28; prof.push([r * k * Math.pow(Math.sin(Math.PI * Math.pow(s, .5)), .85) * (1 - s * .1) + 1e-4, s * h * (k * .25 + .75) * (1 + .06 * Math.sin(ph) * k)]); }
    const g = lathe(prof, 20), p = g.attributes.position, col = new Float32Array(p.count * 3), cc = c.clone().lerp(new THREE.Color(1, 1, 1), wh);
    for (let i = 0; i < p.count; i++) {
      const s = p.getY(i) / h, f = Math.pow(Math.max(0, 1 - s), .7) * Math.min(1, s * 9 + .25) * pw;
      col[i * 3] = cc.r * f + (s < .12 ? .05 : 0); col[i * 3 + 1] = cc.g * f + (s < .12 ? .1 : 0); col[i * 3 + 2] = cc.b * f + (s < .12 ? .35 : 0);
      p.setX(i, p.getX(i) + Math.sin(ph + s * 3) * r * .5 * s * s); p.setZ(i, p.getZ(i) + Math.cos(ph * 2 + s * 2) * r * .25 * s * s);
    }
    g.setAttribute('color', new THREE.BufferAttribute(col, 3)); out.push(new THREE.Mesh(g, mk()));
  }
  return out;
}

// The warm bloom a flame puts into whatever is close to it: an added layer that
// is brightest at the flame and gone a little way off. It belongs to the light.
export function lampGlow(mesh, at, hex, power, reach) {
  const g = mesh.geometry.clone(), p = g.attributes.position, col = new Float32Array(p.count * 3), c = new THREE.Color(hex), v = new THREE.Vector3(), o = V(at[0], at[1], at[2]);
  for (let i = 0; i < p.count; i++) { const f = power * Math.exp(-v.fromBufferAttribute(p, i).distanceTo(o) / reach); col[i * 3] = c.r * f; col[i * 3 + 1] = c.g * f; col[i * 3 + 2] = c.b * f; }
  g.setAttribute('color', new THREE.BufferAttribute(col, 3));
  const m = new THREE.Mesh(g, new THREE.MeshBasicMaterial({ vertexColors: true, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false })); m.userData.glow = true; m.userData.nofit = true; m.userData.noshadow = true; m.renderOrder = 1; return m;
}

// A grip bound in cord: a core turned to rf(y) between y0 and y1 with a cord wound round it.
export function wrap(y0, y1, rf, mat, o) {
  o = o || {}; const core = [], cord = [], turns = o.turns || 30, per = 16, sz = o.sz || 1, cr = o.cord || .0075;
  for (let i = 0; i <= 24; i++) { const y = y0 + (y1 - y0) * i / 24; core.push([rf(y), y]); }
  if (y1 < y0) core.reverse();
  for (let i = 0; i <= turns * per; i++) { const a = i / per * Math.PI * 2, y = y0 + (y1 - y0) * i / (turns * per), r = rf(y) + cr * .5; cord.push(V(Math.cos(a) * r, y, Math.sin(a) * r * sz)); }
  return [M(lathe(core, 28), mat, { s: [1, 1, sz] }), M(tube(cord, cr, 6), mat)];
}
// Straps and bands round a turned body: a ring of radius r at height y.
export function band(r, y, w, t, mat, o) { return turned([[r, y - w / 2], [r + t, y - w / 2 + t * .4], [r + t, y + w / 2 - t * .4], [r, y + w / 2]], mat, Object.assign({ sm: 0, seg: 48 }, o || {})); }

// A domed plate: an outline filled in toward its middle and lifted by zf(x, y).
// outline is a closed list of [x, y]; the plate is one-sided unless the material says otherwise.
export function fan(outline, zf, mat, o) {
  o = o || {}; const c = new THREE.CatmullRomCurve3(outline.map(p => V(p[0], p[1], 0)), true, 'catmullrom', o.tension == null ? .5 : o.tension), nu = o.nu || 96, nv = o.nv || 24, cx = o.cx || 0, cy = o.cy || 0, ring0 = c.getSpacedPoints(nu);
  return sheet((u, v) => { const q = ring0[Math.min(nu, Math.round(u * nu)) % nu], x = cx + (q.x - cx) * v, y = cy + (q.y - cy) * v; return V(x, y, zf(x, y, v)); }, nu, nv, mat, o);
}
// The same outline as a raised edge: a rod laid along it, following the dome.
export function rim(outline, zf, radius, mat, o) {
  o = o || {}; const c = new THREE.CatmullRomCurve3(outline.map(p => V(p[0], p[1], 0)), true, 'catmullrom', o.tension == null ? .5 : o.tension), pts = c.getSpacedPoints(o.n || 120); pts.pop();
  pts.forEach(q => { q.z = zf(q.x, q.y, 1) + (o.lift || 0); });
  return M(tube(pts, radius, o.sides || 10, true), mat, o);
}
// A cut stone as one closed solid: table and crown on top, pavilion falling to a point below. Axis +Z.
export function cutStone(R, crown, pav) {
  const T = [], G = [], pos = [], tr = R * .56, tri = (a, b, c) => pos.push(a.x, a.y, a.z, b.x, b.y, b.z, c.x, c.y, c.z);
  for (let k = 0; k < 8; k++) { const a = (k + .5) * Math.PI / 4; T.push(V(Math.cos(a) * tr, Math.sin(a) * tr, crown)); }
  for (let j = 0; j < 16; j++) { const a = (j + 1) * Math.PI / 8; G.push(V(Math.cos(a) * R, Math.sin(a) * R, 0)); }
  const top = V(0, 0, crown), cul = V(0, 0, -pav);
  for (let k = 0; k < 8; k++) { const t0 = T[k], t1 = T[(k + 1) % 8], g0 = G[(2 * k) % 16], g1 = G[(2 * k + 1) % 16], g2 = G[(2 * k + 2) % 16]; tri(top, t0, t1); tri(t0, g0, g1); tri(t0, g1, t1); tri(t1, g1, g2); }
  for (let j = 0; j < 16; j++) { const k = (j + 1) % 16, m0 = G[j].clone().multiplyScalar(.5).setZ(-pav * (j % 2 ? .6 : .78)), m1 = G[k].clone().multiplyScalar(.5).setZ(-pav * (k % 2 ? .6 : .78)); tri(G[k], G[j], m0); tri(G[k], m0, m1); tri(m1, m0, cul); }
  const g = new THREE.BufferGeometry(); g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3)); g.computeVertexNormals(); return g;
}
// A plume or tassel: many fine strands from a root, falling along a direction.
export function strands(n, rootFn, endFn, radius, mat, seed) {
  const r = rng(seed || 2), out = [];
  for (let i = 0; i < n; i++) { const a = rootFn(r, i), b = endFn(r, i, a), mid = a.clone().lerp(b, .5).add(V((r() - .5) * .06, .06 + r() * .08, (r() - .5) * .06)); out.push(M(tube(path([a, mid, b], 8), radius, 5), mat)); }
  return out;
}

// A flat bar swept along a curve lying in the XY plane, with crisp edges: pick heads, tong jaws, hinges.
export function bar(pts, wf, tf, mat, o) {
  const P = pts.map(p => p instanceof THREE.Vector3 ? p : V(p[0], p[1], p[2])), n = P.length, rings = [];
  for (let i = 0; i < n; i++) { const a = P[Math.max(0, i - 1)], b = P[Math.min(n - 1, i + 1)], t = b.clone().sub(a).normalize(), nn = V(-t.y, t.x, 0), s = i / (n - 1), w = (typeof wf === 'function' ? wf(s) : wf) / 2, th = (typeof tf === 'function' ? tf(s) : tf) / 2; rings.push([P[i].clone().addScaledVector(nn, w).add(V(0, 0, th)), P[i].clone().addScaledVector(nn, -w).add(V(0, 0, th)), P[i].clone().addScaledVector(nn, -w).add(V(0, 0, -th)), P[i].clone().addScaledVector(nn, w).add(V(0, 0, -th))]); }
  const g = loft(rings, { sharp: true }), cap = (r, flip) => { const c = r[0].clone().add(r[1]).add(r[2]).add(r[3]).multiplyScalar(.25); return loft([r, r.map(() => c)], { sharp: true, flip: flip }); };
  const m = M(g, mat, o); m.userData.caps = [M(cap(rings[0], true), mat, o), M(cap(rings[n - 1], false), mat, o)]; return [m].concat(m.userData.caps);
}
// A drawn surface: paper, a dial, a label. draw(ctx, w, h) paints it on a parchment ground.
import { tex } from './util.js';
export function drawn(w, h, draw, ground) {
  const t = tex(w, h, () => ground || [.9, .86, .74], { srgb: true, draw: draw }); t.wrapS = t.wrapT = THREE.ClampToEdgeWrapping; return t;
}
