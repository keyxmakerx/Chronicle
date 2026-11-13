// The picture room: a dark table, soft boxes for the metal to mirror, a hard
// warm key light and a cold light on the far edge. A still is the average of
// many renders with the lights moved a little, which is what softens shadows.
import * as THREE from 'three';
import { rng } from './util.js';
import { ITEMS, DEMO } from './items/index.js';

const D = Math.PI / 180;
export const PRESETS = {
  painted: { ground: 0x120d0a, el: 52, elStand: 15, fov: 24, key: [0xffc98a, 3.8, [-.75, .85, .35], .16], rim: [0x5a86ff, 3.2, [.75, .4, -.7]], sky: .28, env: 'dark', envI: .6, exposure: 1.12, vignette: .8, back: [9, 7, 6] },
  studio: { ground: 0x1c1a19, el: 57, elStand: 18, fov: 24, key: [0xfff0dc, 3.1, [-.55, 1, .5], .3], rim: [0xbcd2ff, 1.5, [.7, .45, -.8]], sky: .55, env: 'dark', envI: .95, exposure: 1.05, vignette: .55, back: [16, 15, 14] },
};

function panel(w, h, pos, colour) {
  const m = new THREE.Mesh(new THREE.PlaneGeometry(w, h), new THREE.MeshBasicMaterial({ color: new THREE.Color(colour[0], colour[1], colour[2]), side: THREE.DoubleSide, toneMapped: false }));
  m.position.set(pos[0], pos[1], pos[2]); m.lookAt(0, 0, 0); return m;
}
export function makeEnv(renderer) {
  const s = new THREE.Scene(), pm = new THREE.PMREMGenerator(renderer);
  s.add(new THREE.Mesh(new THREE.SphereGeometry(30, 32, 16), new THREE.MeshBasicMaterial({ color: new THREE.Color(.035, .035, .036), side: THREE.BackSide, toneMapped: false })));
  s.add(panel(7, 5, [-6, 8, 5], [15, 14, 12.6])); s.add(panel(1.2, 12, [8, 3.5, -5], [7, 8.5, 11])); s.add(panel(9, 3, [1, 1.2, 10], [.5, .46, .42]));
  s.add(panel(3, 3, [5, 9, 4], [2.2, 2.1, 2])); s.add(panel(40, 40, [0, -6, 0], [.05, .045, .04]));
  s.add(panel(10, 7, [3, 2.5, 11], [1.0, .95, .88])); s.add(panel(6, 8, [-10, 2, 4], [1.9, 1.75, 1.5]));
  s.add(panel(12, 7, [0, 10, -7.5], [1.05, 1.03, 1.0])); s.add(panel(3.4, 1.1, [-2.5, 10.5, -5], [5.5, 5.2, 4.8]));
  const rt = pm.fromScene(s, .02); pm.dispose(); return rt.texture;
}

export function buildItem(key, recipe, t) { const d = DEMO[key] || {}; return ITEMS[key](Object.assign({ body: '#9aa3ad', trim: '#b98a3a', detail: '#7a2a22' }, d, recipe || {}), t || 0); }

function measure(group) {
  const fit = new THREE.Box3(); group.updateMatrixWorld(true);
  group.traverse(m => { if (!m.isMesh || m.userData.nofit) return; if (m.isInstancedMesh) m.computeBoundingBox(); else if (!m.geometry.boundingBox) m.geometry.computeBoundingBox(); fit.union((m.isInstancedMesh ? m.boundingBox : m.geometry.boundingBox).clone().applyMatrix4(m.matrixWorld)); });
  return fit;
}

// One finished still as a 2D canvas.
// opts: size, samples, apart (0..1), pass ('full' | 'base' | 'light'), cut (clear background), yaw, t.
export function still(renderer, envs, key, recipe, preset, opts) {
  opts = opts || {};
  const p = PRESETS[preset || 'painted'], size = opts.size || 512, samples = opts.samples || 32, r = rng(opts.seed || 3), pass = opts.pass || 'full', cut = !!opts.cut;
  const scene = new THREE.Scene(); envs.e = envs.e || makeEnv(renderer);
  scene.environment = envs.e; scene.environmentIntensity = pass === 'light' ? 0 : p.envI;
  // The table runs back and curves up into a wall, so a standing item has no horizon behind it.
  let ground;
  if (cut) { ground = new THREE.Mesh(new THREE.PlaneGeometry(600, 600), new THREE.ShadowMaterial({ opacity: .62 })); ground.rotation.x = -Math.PI / 2; }
  else {
    const prof = [[300, 0]], zc = -2.6, R = 3.2; for (let i = 0; i <= 24; i++) { const a = i / 24 * Math.PI / 2; prof.push([zc - Math.sin(a) * R, R - Math.cos(a) * R]); } prof.push([zc - R, 300]);
    const pos = [], idx = []; prof.forEach(([z, y], i) => { pos.push(-300, y, z, 300, y, z); if (i) { const k = i * 2; idx.push(k - 2, k - 1, k, k - 1, k + 1, k); } });
    const cg = new THREE.BufferGeometry(); cg.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3)); cg.setIndex(idx); cg.computeVertexNormals();
    ground = new THREE.Mesh(cg, new THREE.MeshStandardMaterial({ color: p.ground, roughness: .82, metalness: 0 }));
  }
  ground.receiveShadow = true; scene.add(ground);
  const mk = (c, i) => { const l = new THREE.DirectionalLight(c, pass === 'light' ? 0 : i); l.castShadow = pass !== 'light'; l.shadow.mapSize.set(2048, 2048); l.shadow.bias = -.0004; l.shadow.normalBias = .004; const sc = l.shadow.camera; sc.left = sc.bottom = -3; sc.right = sc.top = 3; sc.near = .5; sc.far = 30; scene.add(l); scene.add(l.target); return l; };
  const keyL = mk(p.key[0], p.key[1]), sky = mk(0xffffff, p.sky), rim = p.rim ? mk(p.rim[0], p.rim[1]) : null;

  const item = buildItem(key, recipe, opts.t), g = item.group, ap = opts.apart || 0;
  if (opts.mask) maskItem(key, recipe, opts.t, item, ground);
  if (ap) for (const c of g.children) if (c.userData.ex) c.position.copy(c.userData.ex).multiplyScalar(ap * (item.spread || 1));
  // Everything is scaled to a common size so one set of lights and shadows suits a ring and a spear alike.
  // A moving item is framed as it stands at rest, so the picture does not shift from frame to frame.
  const rest = opts.t ? buildItem(key, recipe, 0) : item, raw = ap || !rest.fit ? measure(g) : rest.fit, k = 2.2 / raw.getSize(new THREE.Vector3()).length();
  const root = new THREE.Group(), tilt = new THREE.Group(); scene.add(root); root.add(tilt); tilt.add(g); g.scale.setScalar(k);
  const c0 = raw.getCenter(new THREE.Vector3()).multiplyScalar(k);
  if (item.stand) g.position.set(-c0.x, -raw.min.y * k + .001, -c0.z);
  else { tilt.rotation.x = -Math.PI / 2 + (item.prop || 0); g.position.set(-c0.x, -c0.y, -raw.min.z * k + .002); }
  root.rotation.y = (item.yaw || 0) + (opts.yaw || 0);
  for (const L of item.lights || []) { if (pass === 'base' || opts.mask) break; const pl = new THREE.PointLight(L.color, L.power * (1 + (L.flick || 0) * Math.sin((opts.t || 0) * Math.PI * 2 * (L.rate || 3) + (L.ph || 0))), (L.reach || 6), 2); pl.position.set(L.p[0], L.p[1], L.p[2] || 0); pl.castShadow = true; pl.shadow.mapSize.set(512, 512); pl.shadow.bias = -.002; pl.shadow.radius = 4; g.add(pl); }
  if (pass === 'light') g.traverse(m => { if (m.isMesh && !m.userData.glow && m.material.emissiveIntensity) m.material.emissiveIntensity = 0; });
  if (pass === 'base') g.traverse(m => { if (m.userData.glow) m.visible = false; });
  scene.updateMatrixWorld(true);
  // Light that has passed through a clear solid gathers on the side away from the lamp.
  const kd0 = new THREE.Vector3().fromArray(p.key[2]).normalize(), inner = [];
  g.traverse(m => { const u = m.isMesh && !m.userData.layered && m.material.userData && m.material.userData.inner; if (u && pass !== 'light') inner.push([m, u]); });
  for (const [m, u] of inner) {
    const geo = m.geometry, nm = geo.attributes.normal, col = new Float32Array(nm.count * 3), c = new THREE.Color(u.hex), n3 = new THREE.Matrix3().getNormalMatrix(m.matrixWorld), v = new THREE.Vector3();
    for (let i = 0; i < nm.count; i++) { v.fromBufferAttribute(nm, i).applyMatrix3(n3).normalize(); const d = -v.dot(kd0), f = Math.pow(Math.max(0, d * .75 + .25), 2.6) * u.power + Math.max(0, v.y) * .05; col[i * 3] = c.r * f; col[i * 3 + 1] = c.g * f; col[i * 3 + 2] = c.b * f; }
    const g2 = geo.clone(); g2.setAttribute('color', new THREE.BufferAttribute(col, 3));
    const ov = new THREE.Mesh(g2, new THREE.MeshBasicMaterial({ vertexColors: true, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false })); ov.renderOrder = (m.renderOrder || 0) + 1; ov.userData.layered = true; m.parent.add(ov);
  }

  renderer.setPixelRatio(1); renderer.setSize(size, size, false);
  renderer.toneMapping = THREE.ACESFilmicToneMapping; renderer.toneMappingExposure = p.exposure; renderer.shadowMap.enabled = true; renderer.shadowMap.type = THREE.PCFShadowMap;
  renderer.setClearColor(cut || pass === 'light' ? 0x000000 : p.ground, cut ? 0 : 1);
  const elv = (opts.el != null ? opts.el : item.el != null ? item.el : item.stand ? p.elStand : p.el) * D;
  const cam = new THREE.PerspectiveCamera(p.fov, 1, .1, 2000), dir = new THREE.Vector3(0, Math.sin(elv), Math.cos(elv));
  let dist = 1.3 / Math.sin(p.fov * D / 2); const focus = new THREE.Vector3(0, .03, 0);
  const corners = [], camR = new THREE.Vector3(1, 0, 0), camU = new THREE.Vector3(0, Math.cos(elv), -Math.sin(elv));
  for (let i = 0; i < 8; i++) corners.push(new THREE.Vector3(i & 1 ? raw.max.x : raw.min.x, i & 2 ? raw.max.y : raw.min.y, i & 4 ? raw.max.z : raw.min.z).applyMatrix4(g.matrixWorld));
  for (let it = 0; it < 6; it++) {
    cam.position.copy(focus).addScaledVector(dir, dist); cam.up.set(0, 1, 0); cam.lookAt(focus); cam.updateMatrixWorld(true);
    let x0 = 9, x1 = -9, y0 = 9, y1 = -9;
    for (const c of corners) { const q = c.clone().project(cam); x0 = Math.min(x0, q.x); x1 = Math.max(x1, q.x); y0 = Math.min(y0, q.y); y1 = Math.max(y1, q.y); }
    const half = Math.tan(p.fov * D / 2) * dist;
    focus.addScaledVector(camR, (x0 + x1) / 2 * half).addScaledVector(camU, (y0 + y1) / 2 * half);
    dist *= Math.max(x1 - x0, y1 - y0) / 2 / (opts.fill || item.fill || .84);
  }
  const eye = focus.clone().addScaledVector(dir, dist), gl = renderer.getContext();
  const sum = new Float32Array(size * size * 4), buf = new Uint8Array(size * size * 4);
  const kd = new THREE.Vector3().fromArray(p.key[2]).normalize(), rd = p.rim ? new THREE.Vector3().fromArray(p.rim[2]).normalize() : null;
  function shoot(n, acc) {
    for (let s = 0; s < n; s++) {
      const ja = r() * Math.PI * 2, jr = Math.sqrt(r()) * p.key[3];
      keyL.position.copy(kd).add(new THREE.Vector3(Math.cos(ja) * jr, 0, Math.sin(ja) * jr)).normalize().multiplyScalar(12);
      const sa = r() * Math.PI * 2, sy = .25 + r() * .75, sr = Math.sqrt(1 - sy * sy); sky.position.set(Math.cos(sa) * sr, sy, Math.sin(sa) * sr).multiplyScalar(12);
      if (rim) rim.position.copy(rd).add(new THREE.Vector3((r() - .5) * .2, (r() - .5) * .2, (r() - .5) * .2)).multiplyScalar(12);
      cam.position.copy(eye); cam.up.set(0, 1, 0); cam.lookAt(focus); cam.setViewOffset(size, size, r() - .5, r() - .5, size, size);
      renderer.render(scene, cam); gl.readPixels(0, 0, size, size, gl.RGBA, gl.UNSIGNED_BYTE, buf);
      for (let i = 0; i < buf.length; i++) acc[i] += buf[i];
    }
  }
  shoot(samples, sum);

  // The halo round anything that gives off light: only the glowing parts, blurred and added back.
  let halo = null; const glowing = []; g.traverse(m => { if (m.userData.glow && m.visible) glowing.push(m); });
  if (glowing.length && pass !== 'base') {
    const black = new THREE.MeshBasicMaterial({ color: 0x000000 }), keep = [];
    scene.traverse(m => { if (!m.isMesh || m.userData.glow) return; keep.push([m, m.material, m.visible]); if (m.material.transparent || m === ground) m.visible = false; else m.material = black; });
    const was = renderer.getClearAlpha(); renderer.setClearColor(0x000000, 1);
    const gs = new Float32Array(size * size * 4), n = 6; shoot(n, gs);
    for (const [m, mat, vis] of keep) { m.material = mat; m.visible = vis; }
    renderer.setClearColor(0x000000, was);
    halo = document.createElement('canvas'); halo.width = halo.height = size; const hc = halo.getContext('2d'), hi = hc.createImageData(size, size);
    for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) { const i = (y * size + x) * 4, j = ((size - 1 - y) * size + x) * 4; hi.data[i] = gs[j] / n; hi.data[i + 1] = gs[j + 1] / n; hi.data[i + 2] = gs[j + 2] / n; hi.data[i + 3] = cut ? Math.max(gs[j], gs[j + 1], gs[j + 2]) / n : 255; }
    hc.putImageData(hi, 0, 0);
  }

  const out = document.createElement('canvas'); out.width = out.height = size;
  const ctx = out.getContext('2d'), im = ctx.createImageData(size, size), d = im.data, cx = size / 2, b = opts.mask ? [0, 0, 0] : p.back, vg = cut || pass === 'light' || opts.mask ? 0 : p.vignette;
  for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
    const i = (y * size + x) * 4, j = ((size - 1 - y) * size + x) * 4;
    const rr = Math.hypot(x - cx, y - cx) / (size * .72), v = Math.min(1, Math.max(0, (rr - .45) / .55)), w = v * v * (3 - 2 * v) * vg;
    if (cut) { const a = sum[j + 3] / samples; d[i + 3] = a; const u = a > 0 ? 255 / a : 0; d[i] = sum[j] / samples * u; d[i + 1] = sum[j + 1] / samples * u; d[i + 2] = sum[j + 2] / samples * u; }
    else { d[i] = sum[j] / samples * (1 - w) + b[0] * w; d[i + 1] = sum[j + 1] / samples * (1 - w) + b[1] * w; d[i + 2] = sum[j + 2] / samples * (1 - w) + b[2] * w; d[i + 3] = 255; }
  }
  ctx.putImageData(im, 0, 0);
  if (halo) {
    ctx.globalCompositeOperation = 'lighter';
    for (const [rad, al] of [[size / 90, .5], [size / 28, .42], [size / 9, .3]]) { ctx.filter = 'blur(' + rad + 'px)'; ctx.globalAlpha = al * (item.halo == null ? 1 : item.halo); ctx.drawImage(halo, 0, 0); }
    ctx.filter = 'none'; ctx.globalAlpha = 1; ctx.globalCompositeOperation = 'source-over';
  }
  scene.traverse(o => { if (o.geometry) o.geometry.dispose(); });
  return out;
}
export { THREE, DEMO, ITEMS };

// The colour-role mask. Every surface a recipe colour reaches is drawn flat
// in that role's channel (body red, trim green, detail blue), as bright as the
// share of its colour that comes from the recipe: a wood base that is mostly
// its own brown and only a little the trim is drawn faint, so a recolour moves
// it only a little. The share is found by building the item again with that
// role black and seeing how far each surface's colour falls.
function cols(m) {
  // What the surface shows: its colour times its vertex and instance colours, and its glow.
  const mat = m.material, d = mat && mat.color ? [mat.color.r, mat.color.g, mat.color.b] : [1, 1, 1];
  const avg = (a, n) => { const s = [0, 0, 0]; for (let i = 0; i < a.length; i++) s[i % 3] += a[i]; return s.map(v => v / Math.max(1, n)); };
  const c = m.geometry && m.geometry.attributes.color;
  if (c && (mat.vertexColors || m.isMesh)) { const v = avg(c.array, c.count); for (let j = 0; j < 3; j++) d[j] *= v[j]; }
  if (m.instanceColor) { const v = avg(m.instanceColor.array, m.instanceColor.array.length / 3); for (let j = 0; j < 3; j++) d[j] *= v[j]; }
  const out = [d];
  if (mat && mat.emissive && mat.emissiveIntensity) out.push([mat.emissive.r, mat.emissive.g, mat.emissive.b].map(v => v * mat.emissiveIntensity));
  return out;
}
// share: how much of the surface's colour went away with the role.
function share(a, b) {
  let num = 0, den = 0;
  for (let k = 0; k < a.length && k < b.length; k++) for (let j = 0; j < 3; j++) { num += Math.max(0, a[k][j] - b[k][j]); den += a[k][j]; }
  return den > 1e-6 ? Math.min(1, num / den) : 0;
}
function meshes(g) { const out = []; g.traverse(m => { if (m.isMesh || m.isLineSegments) out.push(m); }); return out; }
function maskItem(key, recipe, t, item, ground) {
  const roles = ['body', 'trim', 'detail'], own = meshes(item.group), ref = own.map(cols);
  const best = own.map(() => [-1, 0]);
  roles.forEach((r, ri) => {
    const alt = meshes(buildItem(key, Object.assign({}, recipe, { [r]: '#000000' }), t).group);
    if (alt.length !== own.length) throw new Error('mask: ' + key + ' builds differently for ' + r);
    alt.forEach((m, i) => {
      let f = share(ref[i], cols(m));
      // A clear solid's colour is all the recipe's, however much white the room puts in it.
      if (f > .02 && own[i].material.userData && own[i].material.userData.layers) f = 1;
      if (f > .02 && f > best[i][1]) best[i] = [ri, f];
    });
  });
  own.forEach((m, i) => {
    const o = m.material, add = o.blending === THREE.AdditiveBlending, [ri, f] = best[i];
    const c = [0, 0, 0]; if (ri >= 0) c[ri] = f;
    m.userData.glow = false;
    if (m.isLineSegments || (add && ri < 0)) { m.visible = false; return; }
    // Light a colour gives off (a flame's edge, a glow) still follows its role, faintly.
    if (add) { m.material = new THREE.MeshBasicMaterial({ color: new THREE.Color().setRGB(c[0] * .25, c[1] * .25, c[2] * .25, THREE.SRGBColorSpace), transparent: true, blending: THREE.AdditiveBlending, depthWrite: false, side: o.side, toneMapped: false }); return; }
    m.material = new THREE.MeshBasicMaterial({ color: new THREE.Color().setRGB(c[0], c[1], c[2], THREE.SRGBColorSpace), transparent: !!o.transparent, opacity: o.opacity == null ? 1 : o.opacity, side: o.side, depthWrite: o.depthWrite, toneMapped: false, fog: false });
  });
  ground.material = ground.material.isShadowMaterial ? new THREE.MeshBasicMaterial({ visible: false }) : new THREE.MeshBasicMaterial({ color: 0x000000, toneMapped: false });
}
